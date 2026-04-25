// Package email is a minimal SMTP wrapper for transactional sends:
// purchase receipts, onboarding welcomes, refund confirmations.
//
// We use the stdlib net/smtp client deliberately — no third-party deps,
// works against any provider that speaks SMTP (SES / Mailgun / Postmark
// in their SMTP modes, plus self-hosted Postfix). Multipart HTML+plain
// is built by hand because the contract is small and templates are few.
//
// Failures are logged + swallowed. We never block a payment-verify
// response on email delivery — a missed receipt becomes a customer-
// support ticket, a 5xx on the API ruins the user's checkout.
package email

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"
	"text/template"
	"time"

	"live-platform/internal/config"
)

// Client is the contract callers depend on. nil = disabled (config didn't
// set SMTP_HOST). Implementations must be safe for concurrent use — we
// fan out from the Kafka consumer where multiple handlers can fire at once.
type Client interface {
	Send(ctx context.Context, to, subject, htmlBody, textBody string) error
	SendTemplate(ctx context.Context, to, templateName string, data any) error
}

// SMTP is the standard implementation. The actual smtp.SendMail call is
// synchronous, but we wrap it in a context-cancellable goroutine so a
// hung SMTP server can't block the caller for the full kernel TCP
// timeout (~minutes on Linux defaults).
type SMTP struct {
	cfg config.EmailConfig
	log *slog.Logger
	tpl *template.Template
}

// New picks the right impl off the config. nil is a valid return —
// callers should treat that as "email disabled" and degrade gracefully.
func New(cfg config.EmailConfig, log *slog.Logger) Client {
	if cfg.Host == "" {
		log.Info("email disabled — SMTP_HOST unset")
		return nil
	}
	tpl := template.Must(template.New("root").Parse(""))
	for name, body := range builtinTemplates {
		template.Must(tpl.New(name).Parse(body))
	}
	return &SMTP{cfg: cfg, log: log, tpl: tpl}
}

// Send delivers a single message. Both htmlBody + textBody are required;
// the multipart builder picks whichever the recipient's client prefers.
func (s *SMTP) Send(ctx context.Context, to, subject, htmlBody, textBody string) error {
	if s == nil {
		return nil
	}
	timeout := time.Duration(s.cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 8 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", s.cfg.Host, s.cfg.Port)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)
	msg := buildMime(s.cfg.FromAddr, to, subject, htmlBody, textBody)

	done := make(chan error, 1)
	go func() {
		done <- smtp.SendMail(addr, auth, extractEmail(s.cfg.FromAddr),
			[]string{to}, msg)
	}()
	select {
	case err := <-done:
		if err != nil {
			s.log.Warn("smtp send failed",
				slog.String("to", to),
				slog.String("err", err.Error()))
			return err
		}
		s.log.Info("smtp sent", slog.String("to", to), slog.String("subject", subject))
		return nil
	case <-ctx.Done():
		s.log.Warn("smtp send timed out", slog.String("to", to))
		return ctx.Err()
	}
}

// SendTemplate renders one of the named templates against `data` and
// sends it. The HTML version is the canonical one; we render a plain-
// text fallback by stripping tags — good enough for receipts.
func (s *SMTP) SendTemplate(ctx context.Context, to, name string, data any) error {
	if s == nil {
		return nil
	}
	subjectKey := name + ".subject"
	htmlKey := name + ".html"
	if s.tpl.Lookup(subjectKey) == nil || s.tpl.Lookup(htmlKey) == nil {
		return fmt.Errorf("unknown email template: %s", name)
	}
	var sub, html bytes.Buffer
	if err := s.tpl.ExecuteTemplate(&sub, subjectKey, data); err != nil {
		return err
	}
	if err := s.tpl.ExecuteTemplate(&html, htmlKey, data); err != nil {
		return err
	}
	return s.Send(ctx, to, sub.String(), html.String(), stripTags(html.String()))
}

func buildMime(from, to, subject, html, text string) []byte {
	boundary := "school-mime-" + fmt.Sprint(time.Now().UnixNano())
	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", subject)
	fmt.Fprint(&b, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprint(&b, "Content-Type: text/plain; charset=UTF-8\r\n\r\n")
	b.WriteString(text)
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s\r\n", boundary)
	fmt.Fprint(&b, "Content-Type: text/html; charset=UTF-8\r\n\r\n")
	b.WriteString(html)
	b.WriteString("\r\n")

	fmt.Fprintf(&b, "--%s--\r\n", boundary)
	return b.Bytes()
}

// extractEmail pulls the bare address out of "Name <addr@host>" so the
// SMTP envelope stays RFC-compliant. SendMail is happy with full names
// in the headers but not in the MAIL FROM command.
func extractEmail(s string) string {
	if i := strings.LastIndex(s, "<"); i >= 0 {
		if j := strings.Index(s[i:], ">"); j >= 0 {
			return s[i+1 : i+j]
		}
	}
	return strings.TrimSpace(s)
}

// stripTags is a deliberately naive HTML→plain-text converter for the
// fallback branch. It collapses `<br>`, `<p>` to newlines and drops
// everything else. We render the HTML side carefully so this is enough.
func stripTags(s string) string {
	r := strings.NewReplacer(
		"<br>", "\n", "<br/>", "\n", "<br />", "\n",
		"</p>", "\n\n", "</tr>", "\n",
	)
	s = r.Replace(s)
	var out strings.Builder
	depth := 0
	for _, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		default:
			if depth == 0 {
				out.WriteRune(r)
			}
		}
	}
	return out.String()
}

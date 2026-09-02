// Package google verifies Google Sign-In ID tokens server-side.
//
// The mobile/web client performs the Google Sign-In handshake and receives
// an ID token (a signed JWT). It forwards that token to our /auth/google
// endpoint. We MUST verify it here rather than trust any client-supplied
// identity fields — otherwise anyone can POST {"email":"victim@gmail.com"}
// and impersonate another user.
//
// Verification steps (per https://developers.google.com/identity/sign-in/web/backend-auth):
//  1. Signature — RS256 against Google's published X.509 certs, matched by `kid`.
//  2. `iss`     — must be accounts.google.com or https://accounts.google.com.
//  3. `aud`     — must be one of our configured OAuth client IDs.
//  4. `exp`     — not expired (enforced by the JWT parser).
//
// Google's certs rotate; we cache them and honour the Cache-Control max-age
// on the response so we re-fetch about as often as Google rotates.
package google

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const certsURL = "https://www.googleapis.com/oauth2/v1/certs"

var validIssuers = map[string]bool{
	"accounts.google.com":         true,
	"https://accounts.google.com": true,
}

// Identity is the verified subset of claims we consume.
type Identity struct {
	Sub           string
	Email         string
	EmailVerified bool
	FullName      string
	Picture       string
}

// Verifier verifies Google ID tokens against a set of accepted audiences
// (OAuth client IDs — typically one each for web, Android and iOS).
type Verifier struct {
	audiences map[string]bool
	http      *http.Client

	mu       sync.RWMutex
	certs    map[string]*rsa.PublicKey
	certsExp time.Time
}

// New builds a Verifier. audiences is the list of OAuth client IDs the
// tokens are allowed to be issued for. An empty list disables Google
// sign-in (Verify always errors) — callers should treat a nil Verifier
// and a zero-audience Verifier the same way.
func New(audiences []string) *Verifier {
	m := make(map[string]bool, len(audiences))
	for _, a := range audiences {
		if a = strings.TrimSpace(a); a != "" {
			m[a] = true
		}
	}
	return &Verifier{
		audiences: m,
		http:      &http.Client{Timeout: 10 * time.Second},
		certs:     map[string]*rsa.PublicKey{},
	}
}

// Enabled reports whether at least one audience is configured.
func (v *Verifier) Enabled() bool { return v != nil && len(v.audiences) > 0 }

// Verify parses and fully validates a Google ID token, returning the
// trusted identity on success.
func (v *Verifier) Verify(ctx context.Context, idToken string) (*Identity, error) {
	if !v.Enabled() {
		return nil, fmt.Errorf("google sign-in not configured")
	}

	var claims struct {
		jwt.RegisteredClaims
		Email         string `json:"email"`
		EmailVerified any    `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
	}

	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
	_, err := parser.ParseWithClaims(idToken, &claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("token missing kid")
		}
		return v.keyForKID(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("invalid google token: %w", err)
	}

	if len(claims.Issuer) == 0 || !validIssuers[claims.Issuer] {
		return nil, fmt.Errorf("unexpected token issuer %q", claims.Issuer)
	}

	audOK := false
	for _, aud := range claims.Audience {
		if v.audiences[aud] {
			audOK = true
			break
		}
	}
	if !audOK {
		return nil, fmt.Errorf("token audience not recognised")
	}

	if claims.Subject == "" || claims.Email == "" {
		return nil, fmt.Errorf("token missing subject or email")
	}

	return &Identity{
		Sub:           claims.Subject,
		Email:         strings.ToLower(claims.Email),
		EmailVerified: truthy(claims.EmailVerified),
		FullName:      claims.Name,
		Picture:       claims.Picture,
	}, nil
}

func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return t == "true"
	default:
		return false
	}
}

func (v *Verifier) keyForKID(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	if time.Now().Before(v.certsExp) {
		if k, ok := v.certs[kid]; ok {
			v.mu.RUnlock()
			return k, nil
		}
	}
	v.mu.RUnlock()

	if err := v.refreshCerts(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	defer v.mu.RUnlock()
	k, ok := v.certs[kid]
	if !ok {
		return nil, fmt.Errorf("no google cert for kid %q", kid)
	}
	return k, nil
}

func (v *Verifier) refreshCerts(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certsURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch google certs: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("google certs endpoint http %d", resp.StatusCode)
	}

	// v1/certs returns { "<kid>": "<PEM x509 cert>", ... }
	var raw map[string]string
	if err := json.Unmarshal(body, &raw); err != nil {
		return fmt.Errorf("decode google certs: %w", err)
	}

	parsed := make(map[string]*rsa.PublicKey, len(raw))
	for kid, certPEM := range raw {
		block, _ := pem.Decode([]byte(certPEM))
		if block == nil {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			continue
		}
		if pk, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			parsed[kid] = pk
		}
	}
	if len(parsed) == 0 {
		return fmt.Errorf("no usable google certs in response")
	}

	ttl := 1 * time.Hour
	if cc := resp.Header.Get("Cache-Control"); cc != "" {
		if idx := strings.Index(cc, "max-age="); idx >= 0 {
			if n, err := strconv.Atoi(strings.SplitN(cc[idx+len("max-age="):], ",", 2)[0]); err == nil && n > 0 {
				ttl = time.Duration(n) * time.Second
			}
		}
	}

	v.mu.Lock()
	v.certs = parsed
	v.certsExp = time.Now().Add(ttl)
	v.mu.Unlock()
	return nil
}

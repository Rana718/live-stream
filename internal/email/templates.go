package email

// Built-in email templates. Add a new template by adding two map entries:
//   `<name>.subject` and `<name>.html`. Plain-text body is rendered by
// stripping tags from the HTML so we don't have to maintain two copies.
//
// Templates use Go's text/template syntax. Data is passed in unmodified;
// callers should pass a struct or map with stable field names.
//
// Templates we use:
//   purchase_receipt — sent after a successful course or subscription buy
//   onboarding_welcome — sent when a tenant self-serve-onboards
//   refund_issued     — sent when admin issues a refund
var builtinTemplates = map[string]string{
	"purchase_receipt.subject": `Receipt for your purchase at {{.TenantName}}`,
	"purchase_receipt.html": `
<div style="font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;max-width:600px;margin:0 auto;padding:24px;color:#0f172a">
  <h2 style="margin:0 0 8px">Hi {{.UserName}},</h2>
  <p>Thanks for your purchase at <strong>{{.TenantName}}</strong>.</p>
  <table style="width:100%;border-collapse:collapse;margin:16px 0;font-size:14px">
    <tr><td style="padding:6px 0;color:#64748b">Course</td><td style="padding:6px 0;text-align:right">{{.CourseTitle}}</td></tr>
    <tr><td style="padding:6px 0;color:#64748b">Amount</td><td style="padding:6px 0;text-align:right">₹{{.AmountRupees}}</td></tr>
    <tr><td style="padding:6px 0;color:#64748b">Order ID</td><td style="padding:6px 0;text-align:right;font-family:monospace">{{.OrderID}}</td></tr>
    <tr><td style="padding:6px 0;color:#64748b">Date</td><td style="padding:6px 0;text-align:right">{{.PaidAt}}</td></tr>
  </table>
  <p>You're enrolled — open the app to start learning.</p>
  <p style="margin-top:24px;color:#94a3b8;font-size:12px">This is a computer-generated receipt. For questions, reply to this email.</p>
</div>`,

	"onboarding_welcome.subject": `Your School portal is ready · Org code {{.OrgCode}}`,
	"onboarding_welcome.html": `
<div style="font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;max-width:600px;margin:0 auto;padding:24px;color:#0f172a">
  <h2 style="margin:0 0 8px">Welcome, {{.AdminName}}!</h2>
  <p>Your tenant <strong>{{.OrgName}}</strong> is live on School. Your students log in with this code:</p>
  <p style="font-size:32px;font-family:monospace;letter-spacing:2px;background:#f1f5f9;padding:16px;text-align:center;border-radius:8px;margin:16px 0">
    {{.OrgCode}}
  </p>
  <p>You're on a 14-day trial. Next steps:</p>
  <ol>
    <li>Sign in at <a href="{{.AppURL}}">{{.AppURL}}</a> with phone {{.AdminPhone}}</li>
    <li>Upload your logo + brand colours in Settings</li>
    <li>Import your existing student roster (CSV)</li>
    <li>Schedule your first live class</li>
  </ol>
  <p>Questions? Just reply to this email — a real human reads it.</p>
</div>`,

	"refund_issued.subject": `Refund processed for your purchase at {{.TenantName}}`,
	"refund_issued.html": `
<div style="font-family:system-ui,-apple-system,'Segoe UI',Roboto,sans-serif;max-width:600px;margin:0 auto;padding:24px;color:#0f172a">
  <h2 style="margin:0 0 8px">Hi {{.UserName}},</h2>
  <p>We've processed a refund of <strong>₹{{.AmountRupees}}</strong> for your purchase at <strong>{{.TenantName}}</strong>.</p>
  <p style="font-size:14px;color:#64748b">It typically takes 5-7 business days to reflect in your account, depending on your bank.</p>
  <p>Reason: {{.Reason}}</p>
  <p style="margin-top:24px;color:#94a3b8;font-size:12px">Order ID: {{.OrderID}}</p>
</div>`,
}

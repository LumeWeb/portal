Dear {{if .FirstName}}{{.FirstName}}{{else}}{{.Email}}{{end}},

You are receiving this email because we received a password reset request for your account. If you did not request a password reset, please ignore this email.

To reset your password, please click the link below:
{{.ResetURL}}

This link will expire in {{.ExpireTime}} hours. If you did not request a password reset, no further action is required.

If you're having trouble clicking the reset link, copy and paste the URL below into your web browser:
{{.ResetURL}}

Thank you for using {{.PortalName}}.

Best regards,
The {{.PortalName}} Team

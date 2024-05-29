Dear {{if .FirstName}}{{.FirstName}}{{else}}{{.Email}}{{end}},

Thank you for registering with {{.PortalName}}. To complete your registration and verify your email address, please go to the following link:

{{.VerificationLink}}

Please note, this link will expire in {{.ExpireTime}}. If you did not initiate this request, please ignore this email or contact our support team for assistance.

Best regards,
The {{.PortalName}} Team

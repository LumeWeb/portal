Dear {{if .FirstName}}{{.FirstName}}{{else}}{{.Email}}{{end}},

Thank you for registering with {{.PortalName}}. To complete your registration and verify your email address, please enter the following verification code in the provided field on our website:

Verification Code: {{.VerificationCode}}

Please note, this code will expire in {{.ExpireTime}}. If you did not initiate this request, please ignore this email or contact our support team for assistance.

Best regards,
The {{.PortalName}} Team

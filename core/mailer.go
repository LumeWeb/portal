package core

import (
	"text/template"
)

const MAILER_SERVICE = "mailer"

const MAILER_TPL_PASSWORD_RESET = "password_reset"
const MAILER_TPL_VERIFY_EMAIL = "verify_email"

type MailerTemplateData = map[string]any

type MailerTemplate interface {
	Subject() *template.Template
	Body() *template.Template
}

type MailerService interface {
	TemplateSend(template string, subjectVars MailerTemplateData, bodyVars MailerTemplateData, to string) error
	TemplateRegister(name string, template MailerTemplate) error

	Service
}

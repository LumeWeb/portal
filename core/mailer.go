package core

const MAILER_SERVICE = "mailer"

type MailerTemplateData = map[string]any

type MailerService interface {
	TemplateSend(template string, subjectVars MailerTemplateData, bodyVars MailerTemplateData, to string) error

	Service
}

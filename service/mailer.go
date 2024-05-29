package service

import (
	"github.com/LumeWeb/portal/core"
	"github.com/LumeWeb/portal/service/mailer"
	"github.com/wneessen/go-mail"
	"strings"
)

var _ core.MailerService = (*Mailer)(nil)

type Mailer struct {
	ctx              core.Context
	client           *mail.Client
	templateRegistry *mailer.TemplateRegistry
}

func (m *Mailer) TemplateSend(template string, subjectVars core.MailerTemplateData, bodyVars core.MailerTemplateData, to string) error {
	email, err := m.templateRegistry.RenderTemplate(template, subjectVars, bodyVars)

	if err != nil {
		return err
	}

	email.SetFrom(m.ctx.Config().Config().Core.Mail.From)
	email.SetTo(to)

	msg, err := email.ToMessage()
	if err != nil {
		return err
	}

	return m.client.DialAndSend(msg)
}

func NewMailerService(ctx *core.Context, templateRegistry *mailer.TemplateRegistry) *Mailer {
	m := &Mailer{
		templateRegistry: templateRegistry,
	}

	ctx.RegisterService(m)
	ctx.OnStartup(func(ctx core.Context) error {
		var options []mail.Option

		cfg := ctx.Config()

		if cfg.Config().Core.Mail.Port != 0 {
			options = append(options, mail.WithPort(cfg.Config().Core.Mail.Port))
		}

		if cfg.Config().Core.Mail.AuthType != "" {
			options = append(options, mail.WithSMTPAuth(mail.SMTPAuthType(strings.ToUpper(cfg.Config().Core.Mail.AuthType))))
		}

		if cfg.Config().Core.Mail.SSL {
			options = append(options, mail.WithSSLPort(true))
		}

		options = append(options, mail.WithUsername(cfg.Config().Core.Mail.Username))
		options = append(options, mail.WithPassword(cfg.Config().Core.Mail.Password))

		client, err := mail.NewClient(cfg.Config().Core.Mail.Host, options...)
		if err != nil {
			return err
		}

		m.client = client
		return nil
	})

	return m
}

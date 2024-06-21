package service

import (
	"github.com/wneessen/go-mail"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service/internal/mailer"
	"strings"
)

var _ core.MailerService = (*Mailer)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.MAILER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewMailerService(NewMailerTemplateRegistry())
		},
	})
}

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

func NewMailerService(templateRegistry *mailer.TemplateRegistry) (*Mailer, []core.ContextBuilderOption, error) {
	m := &Mailer{
		templateRegistry: templateRegistry,
	}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			m.ctx = ctx
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
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
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			err := m.client.Close()
			if err != nil && err != mail.ErrNoActiveConnection {
				return err
			}

			return nil
		}),
	)

	return m, opts, nil
}
func NewMailerTemplateRegistry() *mailer.TemplateRegistry {
	return mailer.NewTemplateRegistry()
}

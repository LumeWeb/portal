package mailer

import (
	"context"

	"git.lumeweb.com/LumeWeb/portal/config"
	"github.com/wneessen/go-mail"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type TemplateData = map[string]interface{}

var Module = fx.Module("mailer",
	fx.Options(
		fx.Provide(NewMailer),
		fx.Provide(NewTemplateRegistry),
		fx.Invoke(func(registry *TemplateRegistry) error {
			return registry.loadTemplates()
		}),
	),
)

type Mailer struct {
	config           *config.Manager
	logger           *zap.Logger
	client           *mail.Client
	templateRegistry *TemplateRegistry
}

func (m *Mailer) TemplateSend(template string, subjectVars TemplateData, bodyVars TemplateData, to string) error {
	email, err := m.templateRegistry.RenderTemplate(template, subjectVars, bodyVars)

	if err != nil {
		return err
	}

	msg, err := email.ToMessage()
	if err != nil {
		return err
	}

	return m.client.DialAndSend(msg)
}

func NewMailer(lc fx.Lifecycle, config *config.Manager, logger *zap.Logger, templateRegistry *TemplateRegistry) (*Mailer, error) {
	m := &Mailer{config: config, logger: logger, templateRegistry: templateRegistry}

	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			var options []mail.Option

			if config.Config().Core.Mail.Port != 0 {
				options = append(options, mail.WithPort(config.Config().Core.Mail.Port))
			}

			if config.Config().Core.Mail.AuthType != "" {
				options = append(options, mail.WithSMTPAuth(mail.SMTPAuthType(config.Config().Core.Mail.AuthType)))
			}

			if config.Config().Core.Mail.SSL {
				options = append(options, mail.WithSSLPort(true))
			}

			options = append(options, mail.WithUsername(config.Config().Core.Mail.Username))
			options = append(options, mail.WithPassword(config.Config().Core.Mail.Password))

			client, err := mail.NewClient(config.Config().Core.Mail.Host, options...)
			if err != nil {
				return err
			}

			m.client = client
			return nil
		},
	})

	return m, nil
}

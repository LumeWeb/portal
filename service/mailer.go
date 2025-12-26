package service

import (
	"io/fs"
	"path"
	"strings"
	"text/template"

	"github.com/wneessen/go-mail"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service/internal/mailer"
	mailerMetrics "go.lumeweb.com/portal/service/internal/mailer"
)

var _ core.MailerService = (*Mailer)(nil)

var ErrEmailTemplateNotFound = mailer.ErrTemplateNotFound

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.MAILER_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewMailerService(NewMailerTemplateRegistry())
		},
		Metrics: mailerMetrics.GetCollectors(),
	})
}

type Mailer struct {
	*core.BaseComponent
	client           *mail.Client
	templateRegistry *mailer.TemplateRegistry
	connDialed       bool
}

func (m *Mailer) ID() string {
	return core.MAILER_SERVICE
}

func NewMailerTemplate(subject *template.Template, body *template.Template) *mailer.EmailTemplate {
	return mailer.NewMailerTemplate(subject, body)
}

func (m *Mailer) TemplateRegistry() *mailer.TemplateRegistry {
	return m.templateRegistry
}

func (m *Mailer) TemplateSend(template string, subjectVars core.MailerTemplateData, bodyVars core.MailerTemplateData, to string) error {
	return core.MetricTrack(
		mailerMetrics.MailerDuration.WithLabelValues(mailerMetrics.LabelOpSend),
		mailerMetrics.MailerFailed.WithLabelValues(mailerMetrics.LabelOpSend),
		func() error {
			email, err := m.templateRegistry.RenderTemplate(template, subjectVars, bodyVars)

			if err != nil {
				return err
			}

			email.SetFrom(m.Config().Config().Core.Mail.From)
			email.SetTo(to)

			msg, err := email.ToMessage()
			if err != nil {
				return err
			}

			err = m.client.DialAndSend(msg)
			if err != nil {
				return err
			}

			m.connDialed = true
			mailerMetrics.EmailsSent.WithLabelValues(mailerMetrics.LabelOpSend).Inc()
			return nil
		},
	)
}

func (m *Mailer) TemplateRegister(name string, template core.MailerTemplate) error {
	return core.MetricTrack(
		mailerMetrics.MailerDuration.WithLabelValues(mailerMetrics.LabelOpRegister),
		mailerMetrics.MailerFailed.WithLabelValues(mailerMetrics.LabelOpRegister),
		func() error {
			m.templateRegistry.RegisterTemplate(name, template)
			mailerMetrics.TemplatesTotal.WithLabelValues(mailerMetrics.LabelOpRegister).Inc()
			return nil
		},
	)
}

func NewMailerService(templateRegistry *mailer.TemplateRegistry) (core.Service, []core.ContextBuilderOption, error) {
	m := &Mailer{
		templateRegistry: templateRegistry,
	}

	opts := core.ContextOptions(
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
			if !m.connDialed {
				return nil
			}
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

func MailerTemplatesFromEmbed(embed fs.FS, prefix string) (map[string]core.MailerTemplate, error) {
	if prefix == "" {
		prefix = mailer.EMAIL_FS_PREFIX
	}

	subjectTemplates, err := fs.Glob(embed, path.Join(prefix, "*_subject*"))
	if err != nil {
		return nil, err
	}

	templates := make(map[string]core.MailerTemplate)

	for _, subjectTemplate := range subjectTemplates {
		// Get relative path from prefix
		relPath := strings.TrimPrefix(subjectTemplate, prefix)
		relPath = strings.TrimPrefix(relPath, "/")

		// Extract template name by removing _subject.tpl suffix
		templateName := strings.TrimSuffix(relPath, "_subject.tpl")

		// Construct body template path
		bodyTemplate := path.Join(prefix, templateName+"_body.tpl")

		subjectContent, err := fs.ReadFile(embed, subjectTemplate)
		if err != nil {
			return nil, err
		}

		subjectTmpl, err := template.New(templateName).Parse(string(subjectContent))
		if err != nil {
			return nil, err
		}

		bodyContent, err := fs.ReadFile(embed, bodyTemplate)
		if err != nil {
			return nil, err
		}

		bodyTmpl, err := template.New(templateName).Parse(string(bodyContent))
		if err != nil {
			return nil, err
		}

		templates[templateName] = NewMailerTemplate(subjectTmpl, bodyTmpl)
	}

	return templates, nil
}

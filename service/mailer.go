package service

import (
	"io/fs"
	"path"
	"strings"
	"text/template"

	"github.com/wneessen/go-mail"
	"go.lumeweb.com/portal/core"
	pkgDNS "go.lumeweb.com/portal/internal/dns"
	"go.lumeweb.com/portal/service/internal/mailer"
	mailerMetrics "go.lumeweb.com/portal/service/internal/mailer"
	"go.uber.org/zap"
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
	mailCfg := m.Config().Config().Core.Mail

	m.Logger().Debug("Sending email",
		zap.String("template", template),
		zap.String("to", to),
		zap.String("host", mailCfg.Host),
		zap.Int("port", mailCfg.Port),
		zap.String("from", mailCfg.From),
	)

	return core.MetricTrack(
		mailerMetrics.MailerDuration.WithLabelValues(mailerMetrics.LabelOpSend),
		mailerMetrics.MailerFailed.WithLabelValues(mailerMetrics.LabelOpSend),
		func() error {
			email, err := m.templateRegistry.RenderTemplate(template, subjectVars, bodyVars)

			if err != nil {
				m.Logger().Error("Failed to render email template",
					zap.String("template", template),
					zap.String("to", to),
					zap.Error(err),
				)
				return err
			}

			email.SetFrom(mailCfg.From)
			email.SetTo(to)

			msg, err := email.ToMessage()
			if err != nil {
				m.Logger().Error("Failed to create email message",
					zap.String("template", template),
					zap.String("to", to),
					zap.String("from", mailCfg.From),
					zap.Error(err),
				)
				return err
			}

			err = m.client.DialAndSend(msg)
			if err != nil {
				m.Logger().Error("Failed to send email",
					zap.String("template", template),
					zap.String("to", to),
					zap.String("host", mailCfg.Host),
					zap.Int("port", mailCfg.Port),
					zap.Bool("ssl", mailCfg.SSL),
					zap.String("auth_type", mailCfg.AuthType),
					zap.String("from", mailCfg.From),
					zap.Error(err),
				)
				return err
			}

			m.connDialed = true
			mailerMetrics.EmailsSent.WithLabelValues(mailerMetrics.LabelOpSend).Inc()
			m.Logger().Debug("Email sent successfully",
				zap.String("template", template),
				zap.String("to", to),
				zap.String("host", mailCfg.Host),
				zap.Int("port", mailCfg.Port),
			)
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
			mailCfg := cfg.Config().Core.Mail

			ctx.Logger().Debug("Initializing mailer service",
				zap.String("host", mailCfg.Host),
				zap.Int("port", mailCfg.Port),
				zap.Bool("ssl", mailCfg.SSL),
				zap.String("auth_type", mailCfg.AuthType),
				zap.String("from", mailCfg.From),
				zap.String("username", mailCfg.Username),
			)

			if mailCfg.Port != 0 {
				options = append(options, mail.WithPort(mailCfg.Port))
			}

			if mailCfg.AuthType != "" {
				options = append(options, mail.WithSMTPAuth(mail.SMTPAuthType(strings.ToUpper(mailCfg.AuthType))))
			}

			if mailCfg.SSL {
				options = append(options, mail.WithSSLPort(true))
			}

			options = append(options, mail.WithUsername(mailCfg.Username))
			options = append(options, mail.WithPassword(mailCfg.Password))

			if mailCfg.Host != "" {
				domain := mailCfg.Host
				if ctx.Config().Config().Core.Domain != "" {
					domain = ctx.Config().Config().Core.Domain
				}
				options = append(options, mail.WithHELO(domain))
			}

			// Add custom dialer if DNS resolver is configured
			dnsResolver := ctx.Config().Config().Core.DNSResolver
			if dnsResolver != "" {
				ctx.Logger().Debug("Configuring mail client with custom DNS resolver",
					zap.String("dns_resolver", dnsResolver),
				)
				options = append(options, mail.WithDialContextFunc(pkgDNS.CustomDialer(dnsResolver)))
			}

			client, err := mail.NewClient(mailCfg.Host, options...)
			if err != nil {
				ctx.Logger().Error("Failed to create mail client",
					zap.String("host", mailCfg.Host),
					zap.Int("port", mailCfg.Port),
					zap.Bool("ssl", mailCfg.SSL),
					zap.String("auth_type", mailCfg.AuthType),
					zap.Error(err),
				)
				return err
			}

			m.client = client

			ctx.Logger().Debug("Mailer service initialized successfully",
				zap.String("host", mailCfg.Host),
				zap.Int("port", mailCfg.Port),
				zap.Bool("ssl", mailCfg.SSL),
			)

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

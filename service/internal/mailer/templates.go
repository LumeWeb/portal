package mailer

import (
	"errors"
	"go.lumeweb.com/portal/core"
	"strings"
	"sync"
	"text/template"
)

const EMAIL_FS_PREFIX = "templates/"

var _ core.MailerTemplate = (*EmailTemplate)(nil)

type EmailTemplate struct {
	subject *template.Template
	body    *template.Template
}

func (et *EmailTemplate) Subject() *template.Template {
	return et.subject
}

func (et *EmailTemplate) Body() *template.Template {
	return et.body
}

func NewMailerTemplate(subject *template.Template, body *template.Template) *EmailTemplate {
	return &EmailTemplate{
		subject: subject,
		body:    body,
	}
}

var ErrTemplateNotFound = errors.New("template not found")

type TemplateRegistry struct {
	templates   map[string]core.MailerTemplate
	templatesMu sync.RWMutex
}

func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates:   make(map[string]core.MailerTemplate),
		templatesMu: sync.RWMutex{},
	}
}

func (tr *TemplateRegistry) RegisterTemplate(name string, template core.MailerTemplate) {
	tr.templatesMu.Lock()
	defer tr.templatesMu.Unlock()
	tr.templates[name] = template
}
func (tr *TemplateRegistry) RenderTemplate(templateName string, subjectVars core.MailerTemplateData, bodyVars core.MailerTemplateData) (*Email, error) {
	tmpl, ok := tr.templates[templateName]
	if !ok {
		return nil, ErrTemplateNotFound
	}

	var subjectBuilder strings.Builder
	err := tmpl.Subject().Execute(&subjectBuilder, subjectVars)
	if err != nil {
		return nil, err
	}

	var bodyBuilder strings.Builder
	err = tmpl.Body().Execute(&bodyBuilder, bodyVars)
	if err != nil {
		return nil, err
	}

	return NewEmail(subjectBuilder.String(), bodyBuilder.String()), nil
}

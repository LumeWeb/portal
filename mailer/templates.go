package mailer

import (
	"embed"
	"errors"
	"io/fs"
	"strings"
	"text/template"
)

const EMAIL_FS_PREFIX = "templates"

const TPL_PASSWORD_RESET = "password_reset.tpl"
const TPL_VERIFY_EMAIL = "verify_email.tpl"

type EmailTemplate struct {
	Subject *template.Template
	Body    *template.Template
}

//go:embed templates/*
var templateFS embed.FS

var ErrTemplateNotFound = errors.New("template not found")

type TemplateRegistry struct {
	templates map[string]EmailTemplate
}

func NewTemplateRegistry() *TemplateRegistry {
	return &TemplateRegistry{
		templates: make(map[string]EmailTemplate),
	}
}

func (tr *TemplateRegistry) loadTemplates() error {
	subjectTemplates, err := fs.Glob(templateFS, EMAIL_FS_PREFIX+"*_subject*")

	if err != nil {
		return err
	}

	for _, subjectTemplate := range subjectTemplates {
		templateName := strings.TrimSuffix(strings.TrimPrefix(subjectTemplate, EMAIL_FS_PREFIX), "_subject.tpl")
		bodyTemplate := strings.TrimSuffix(subjectTemplate, "_subject.tpl") + "_body.tpl"

		subjectContent, err := fs.ReadFile(templateFS, templateName+"_subject.tpl")
		if err != nil {
			return err
		}

		subjectTmpl, err := template.New(templateName).Parse(string(subjectContent))
		if err != nil {
			return err
		}

		bodyContent, err := fs.ReadFile(templateFS, bodyTemplate)
		if err != nil {
			return err
		}

		bodyTmpl, err := template.New(templateName).Parse(string(bodyContent))
		if err != nil {
			return err
		}

		tr.templates[templateName] = EmailTemplate{
			Subject: subjectTmpl,
			Body:    bodyTmpl,
		}
	}

	return nil
}

func (tr *TemplateRegistry) RenderTemplate(templateName string, subjectVars TemplateData, bodyVars TemplateData) (*Email, error) {
	tmpl, ok := tr.templates[templateName]
	if !ok {
		return nil, ErrTemplateNotFound
	}

	var subjectBuilder strings.Builder
	err := tmpl.Subject.Execute(&subjectBuilder, subjectVars)
	if err != nil {
		return nil, err
	}

	var bodyBuilder strings.Builder
	err = tmpl.Body.Execute(&bodyBuilder, bodyVars)
	if err != nil {
		return nil, err
	}

	return NewEmail(subjectBuilder.String(), bodyBuilder.String()), nil
}

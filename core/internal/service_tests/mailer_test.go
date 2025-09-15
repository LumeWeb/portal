package service_tests

import (
	"embed"

	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/service"

	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed mailer/testdata/*.tpl
var testTemplates embed.FS

func TestMailerTemplatesFromEmbed(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		templates, err := service.MailerTemplatesFromEmbed(testTemplates, "mailer/testdata/")
		require.NoError(tb, err)
		assert.Len(tb, templates, 2)

		// Check if the templates are parsed correctly
		tmpl1, ok := templates["test_template1"]
		assert.True(tb, ok)
		assert.NotNil(tb, tmpl1.Subject())
		assert.NotNil(tb, tmpl1.Body())

		tmpl2, ok := templates["test_template2"]
		assert.True(tb, ok)
		assert.NotNil(tb, tmpl2.Subject())
		assert.NotNil(tb, tmpl2.Body())

		// Check template content
		var subjectData = core.MailerTemplateData{"Name": "Test"}
		var bodyData = core.MailerTemplateData{"Content": "Test Content"}

		var subjectBuilder strings.Builder
		err = tmpl1.Subject().Execute(&subjectBuilder, subjectData)
		require.NoError(tb, err)
		assert.Equal(tb, "Subject: Test", subjectBuilder.String())

		var bodyBuilder strings.Builder
		err = tmpl1.Body().Execute(&bodyBuilder, bodyData)
		require.NoError(tb, err)
		assert.Equal(tb, "Body: Test Content", bodyBuilder.String())

		var subjectBuilder2 strings.Builder
		err = tmpl2.Subject().Execute(&subjectBuilder2, subjectData)
		require.NoError(tb, err)
		assert.Equal(tb, "Subject2: Test", subjectBuilder2.String())

		var bodyBuilder2 strings.Builder
		err = tmpl2.Body().Execute(&bodyBuilder2, bodyData)
		require.NoError(tb, err)
		assert.Equal(tb, "Body2: Test Content", bodyBuilder2.String())

	},
		coreTesting.WithServiceFactory(core.MAILER_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return service.NewMailerService(service.NewMailerTemplateRegistry())
		}),
		coreTesting.WithConfig("core.mail.host", "localhost"),
	)
}

func TestTemplateRegistry_RegisterAndRenderTemplate(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		mailerService := core.GetService[*service.Mailer](ctx, core.MAILER_SERVICE)
		require.NotNil(tb, mailerService)

		templateRegistry := mailerService.TemplateRegistry()

		// Create a simple template
		subjectTmpl, err := template.New("test_subject").Parse("Subject: {{.Name}}")
		require.NoError(tb, err)
		bodyTmpl, err := template.New("test_body").Parse("Body: {{.Content}}")
		require.NoError(tb, err)

		emailTemplate := service.NewMailerTemplate(subjectTmpl, bodyTmpl)

		// Register the template
		templateRegistry.RegisterTemplate("test_template", emailTemplate)

		// Render the template
		subjectData := core.MailerTemplateData{"Name": "Test"}
		bodyData := core.MailerTemplateData{"Content": "Test Content"}
		email, err := templateRegistry.RenderTemplate("test_template", subjectData, bodyData)
		require.NoError(tb, err)

		// Check the rendered content
		assert.Equal(tb, "Subject: Test", email.Subject())
		assert.Equal(tb, "Body: Test Content", email.Body())

		// Test template not found
		_, err = templateRegistry.RenderTemplate("nonexistent_template", subjectData, bodyData)
		assert.ErrorIs(tb, err, service.ErrEmailTemplateNotFound)

	},
		coreTesting.WithServiceFactory(core.MAILER_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return service.NewMailerService(service.NewMailerTemplateRegistry())
		}),
		coreTesting.WithConfig("core.mail.host", "localhost"))
}

func TestMailerTemplatesFromEmbed_EmptyPrefix(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create a mock embed.FS with some test templates
		testFS := createTestFS(map[string]string{
			"templates/test_template1_subject.tpl": "Subject: {{.Name}}",
			"templates/test_template1_body.tpl":    "Body: {{.Content}}",
		})

		templates, err := service.MailerTemplatesFromEmbed(testFS, "")
		require.NoError(tb, err)
		assert.Len(tb, templates, 1)

		tmpl1, ok := templates["test_template1"]
		assert.True(tb, ok)
		assert.NotNil(tb, tmpl1.Subject())
		assert.NotNil(tb, tmpl1.Body())

		var subjectData = core.MailerTemplateData{"Name": "Test"}
		var bodyData = core.MailerTemplateData{"Content": "Test Content"}

		var subjectBuilder strings.Builder
		err = tmpl1.Subject().Execute(&subjectBuilder, subjectData)
		require.NoError(tb, err)
		assert.Equal(tb, "Subject: Test", subjectBuilder.String())

		var bodyBuilder strings.Builder
		err = tmpl1.Body().Execute(&bodyBuilder, bodyData)
		require.NoError(tb, err)
		assert.Equal(tb, "Body: Test Content", bodyBuilder.String())

	},
		coreTesting.WithServiceFactory(core.MAILER_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return service.NewMailerService(service.NewMailerTemplateRegistry())
		}),
		coreTesting.WithConfig("core.mail.host", "localhost"))
}

func TestMailerTemplatesFromEmbed_NoTemplates(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// Create a mock embed.FS with no templates
		testFS := createTestFS(map[string]string{})

		templates, err := service.MailerTemplatesFromEmbed(testFS, "templates/")
		require.NoError(tb, err)
		assert.Len(tb, templates, 0)

	},
		coreTesting.WithServiceFactory(core.MAILER_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return service.NewMailerService(service.NewMailerTemplateRegistry())
		}),
		coreTesting.WithConfig("core.mail.host", "localhost"))
}

// createTestFS creates a fstest.MapFS from a map of file paths to content.
func createTestFS(files map[string]string) fstest.MapFS {
	fsys := make(fstest.MapFS)
	for name, content := range files {
		fsys[name] = &fstest.MapFile{
			Data: []byte(content),
		}
	}
	return fsys
}

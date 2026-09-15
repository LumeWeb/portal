package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMailConfigDefaults(t *testing.T) {
	defaults := MailConfig{}.Defaults()

	assert.Equal(t, "", defaults["AdminEmail"], "admin_email should default to empty")
	assert.Equal(t, "", defaults["Host"])
	assert.Equal(t, 25, defaults["Port"])
	assert.Equal(t, false, defaults["SSL"])
}

func TestMailConfigAdminEmailEnvVar(t *testing.T) {
	assert.Equal(t, "PORTAL__CORE__MAIL__ADMIN_EMAIL", EnvVarFor("core.mail.admin_email"))
}

func TestMailConfigLoadAdminEmail(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mail_config_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, CoreConfigFile)
	err = os.WriteFile(configFile, []byte(`
domain: example.com
portal_name: mail_config_test
port: 8080
mail:
  admin_email: admin@example.com
`), 0644)
	require.NoError(t, err)

	m, err := NewManager(WithConfigPaths([]string{tempDir}))
	require.NoError(t, err)

	err = m.Init()
	require.NoError(t, err)

	cfg := m.Config()
	require.NotNil(t, cfg)
	assert.Equal(t, "admin@example.com", cfg.Core.Mail.AdminEmail)
}

func TestMailConfigLoadAdminEmailDefault(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "mail_config_default_test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	configFile := filepath.Join(tempDir, CoreConfigFile)
	err = os.WriteFile(configFile, []byte(`
domain: example.com
portal_name: mail_config_default_test
port: 8080
`), 0644)
	require.NoError(t, err)

	m, err := NewManager(WithConfigPaths([]string{tempDir}))
	require.NoError(t, err)

	err = m.Init()
	require.NoError(t, err)

	cfg := m.Config()
	require.NotNil(t, cfg)
	assert.Equal(t, "", cfg.Core.Mail.AdminEmail, "admin_email should default to empty when unset")
}

package config

import (
	"errors"
)

var _ Validator = (*MailConfig)(nil)
var _ Defaults = (*MailConfig)(nil)

type MailConfig struct {
	Host     string `config:"host"`
	Port     int    `config:"port"`
	SSL      bool   `config:"ssl"`
	AuthType string `config:"auth_type"`
	Username string `config:"username"`
	Password string `config:"password"`
	From     string `config:"from"`
}

func (m MailConfig) Validate() error {
	if m.Host == "" {
		return errors.New("core.mail.host is required")
	}
	if m.Username == "" {
		return errors.New("core.mail.username is required")
	}
	if m.Password == "" {
		return errors.New("core.mail.password is required")
	}
	if m.From == "" {
		return errors.New("core.mail.from is required")
	}
	return nil
}
func (c MailConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"host":      "",
		"auth_type": "plain",
		"port":      25,
		"ssl":       false,
		"from":      "",
		"username":  "",
		"password":  "",
	}
}

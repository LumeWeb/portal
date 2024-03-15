package config

import "errors"

var _ Validator = (*MailConfig)(nil)

type MailConfig struct {
	Host     string `mapstructure:"host"`
	Port     int    `mapstructure:"port"`
	SSL      bool   `mapstructure:"ssl"`
	AuthType string `mapstructure:"auth_type"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
	From     string `mapstructure:"from"`
}

func (m MailConfig) Validate() error {
	if m.Host == "" {
		return errors.New("host is required")
	}
	if m.Username == "" {
		return errors.New("username is required")
	}
	if m.Password == "" {
		return errors.New("password is required")
	}
	if m.From == "" {
		return errors.New("from is required")
	}
	return nil
}

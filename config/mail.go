package config

import "errors"

var _ Validator = (*MailConfig)(nil)

type MailConfig struct {
	Host     string
	Port     int
	SSL      bool
	AuthType string
	Username string
	Password string
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
	return nil
}

package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*MailConfig)(nil)
	_ Defaults                           = (*MailConfig)(nil)
)

type MailConfig struct {
	Host     string `config:"host"`
	Port     int    `config:"port"`
	SSL      bool   `config:"ssl"`
	AuthType string `config:"auth_type"`
	Username string `config:"username"`
	Password string `config:"password"`
	From     string `config:"from"`
}

func (m MailConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"host": z.String().
			Required(z.Message("core.mail.host is required")),
		"username": z.String().
			Required(z.Message("core.mail.username is required")),
		"password": z.String().
			Required(z.Message("core.mail.password is required")),
		"from": z.String().
			Required(z.Message("core.mail.from is required")),
		"port": z.Int().
			Default(25),
		"auth_type": z.String().
			Default("plain"),
		"ssl": z.Bool().
			Default(false),
	})
}

func (m MailConfig) Defaults() map[string]interface{} {
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

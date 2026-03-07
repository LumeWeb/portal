package config

import (
	"github.com/Oudwins/zog"
	"github.com/wneessen/go-mail"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*MailConfig)(nil)
	_ Defaults                           = (*MailConfig)(nil)
)

type MailConfig struct {
	Host      string                 `config:"host"`
	Port      int                    `config:"port"`
	SSL       bool                   `config:"ssl"`
	TLSPolicy mail.TLSPolicy         `config:"tls_policy"`
	AuthType  string                 `config:"auth_type"`
	Username  string                 `config:"username"`
	Password  string                 `config:"password"`
	From      string                 `config:"from"`
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
		"port": z.Int(),
		"auth_type": z.String(),
		"ssl": z.Bool(),
		"tls_policy": z.Enum(string(mail.NoTLS), string(mail.TLSOpportunistic), string(mail.TLSMandatory)),
	})
}

func (m MailConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"host":       "",
		"auth_type":  "plain",
		"port":       25,
		"ssl":        false,
		"tls_policy": string(mail.TLSMandatory),
		"from":       "",
		"username":   "",
		"password":   "",
	}
}

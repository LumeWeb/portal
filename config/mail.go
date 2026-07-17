package config

import (
	z "github.com/Oudwins/zog"
	"github.com/wneessen/go-mail"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*MailConfig)(nil)
	_ Defaults                           = (*MailConfig)(nil)
)

// TLS policy string constants - these must match the values returned by mail.TLSPolicy.String()
const (
	TLSPolicyNoTLS         = "NoTLS"
	TLSPolicyOpportunistic = "TLSOpportunistic"
	TLSPolicyMandatory     = "TLSMandatory"
)

// SMTP auth type alias constant - provides a user-friendly alias for NOAUTH
const SMTPAuthAliasNone = "none"

type MailConfig struct {
	Host      string                 `config:"host"`
	Port      int                    `config:"port"`
	SSL       bool                   `config:"ssl"`
	TLSPolicy string                 `config:"tls_policy"`
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
		"auth_type": z.String().
			OneOf([]string{
				SMTPAuthAliasNone,
				string(mail.SMTPAuthCramMD5),
				string(mail.SMTPAuthCustom),
				string(mail.SMTPAuthLogin),
				string(mail.SMTPAuthLoginNoEnc),
				string(mail.SMTPAuthNoAuth),
				string(mail.SMTPAuthPlain),
				string(mail.SMTPAuthPlainNoEnc),
				string(mail.SMTPAuthXOAUTH2),
				string(mail.SMTPAuthSCRAMSHA1),
				string(mail.SMTPAuthSCRAMSHA1PLUS),
				string(mail.SMTPAuthSCRAMSHA256),
				string(mail.SMTPAuthSCRAMSHA256PLUS),
				string(mail.SMTPAuthAutoDiscover),
			}, z.Message("auth_type must be a valid SMTP auth type")),
		"ssl": z.Bool(),
		"tls_policy": z.String().
			OneOf([]string{TLSPolicyNoTLS, TLSPolicyOpportunistic, TLSPolicyMandatory}, z.Message("tls_policy must be one of: NoTLS, TLSOpportunistic, TLSMandatory")),
	})
}

func (m MailConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"Host":      "",
		"AuthType":  string(mail.SMTPAuthPlain),
		"Port":      25,
		"SSL":       false,
		"TLSPolicy": mail.TLSMandatory.String(),
		"From":      "",
		"Username":  "",
		"Password":  "",
	}
}

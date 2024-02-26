package config

type MailConfig struct {
	Host     string
	Port     int
	SSL      bool
	AuthType string
	Username string
	Password string
}

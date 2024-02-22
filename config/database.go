package config

type DatabaseConfig struct {
	Charset  string `mapstructure:"charset"`
	Host     string `mapstructure:"host"`
	Name     string `mapstructure:"name"`
	Password string `mapstructure:"password"`
	Port     int    `mapstructure:"port"`
	Username string `mapstructure:"username"`
}

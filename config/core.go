package config

type CoreConfig struct {
	DB              DatabaseConfig `mapstructure:"db"`
	Domain          string         `mapstructure:"domain"`
	ExternalPort    uint           `mapstructure:"external_port"`
	Identity        string         `mapstructure:"identity"`
	Log             LogConfig      `mapstructure:"log"`
	Port            uint           `mapstructure:"port"`
	PostUploadLimit uint64         `mapstructure:"post_upload_limit"`
	Sia             SiaConfig      `mapstructure:"sia"`
	Storage         StorageConfig  `mapstructure:"storage"`
	Protocols       []string       `mapstructure:"protocols"`
	Mail            MailConfig     `mapstructure:"mail"`
}

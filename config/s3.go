package config

type S3Config struct {
	BufferBucket string `mapstructure:"buffer_bucket"`
	Endpoint     string `mapstructure:"endpoint"`
	Region       string `mapstructure:"region"`
	AccessKey    string `mapstructure:"access_key"`
	SecretKey    string `mapstructure:"secret_key"`
}

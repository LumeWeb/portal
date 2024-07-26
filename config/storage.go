package config

type StorageConfig struct {
	S3  S3Config  `mapstructure:"s3"`
	Sia SiaConfig `mapstructure:"sia"`
	Tus TusConfig `mapstructure:"tus"`
}

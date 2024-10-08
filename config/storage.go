package config

type StorageConfig struct {
	S3  S3Config  `config:"s3"`
	Sia SiaConfig `config:"sia"`
	Tus TusConfig `config:"tus"`
}

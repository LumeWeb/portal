package config

import "errors"

var _ Validator = (*DatabaseConfig)(nil)

type S3Config struct {
	BufferBucket string `mapstructure:"buffer_bucket"`
	Endpoint     string `mapstructure:"endpoint"`
	Region       string `mapstructure:"region"`
	AccessKey    string `mapstructure:"access_key"`
	SecretKey    string `mapstructure:"secret_key"`
}

func (s S3Config) Validate() error {
	if s.BufferBucket == "" {
		return errors.New("core.storage.s3.buffer_bucket is required")
	}
	if s.Endpoint == "" {
		return errors.New("core.storage.s3.endpoint is required")
	}
	if s.Region == "" {
		return errors.New("core.storage.s3.region is required")
	}
	if s.AccessKey == "" {
		return errors.New("core.storage.s3.access_key is required")
	}
	if s.SecretKey == "" {
		return errors.New("core.storage.s3.secret_key is required")
	}
	return nil
}

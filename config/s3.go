package config

import (
	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*S3Config)(nil)
	_ Defaults                           = (*S3Config)(nil)
)

type S3Config struct {
	BufferBucket string `config:"buffer_bucket"`
	Endpoint     string `config:"endpoint"`
	Region       string `config:"region"`
	AccessKey    string `config:"access_key"`
	SecretKey    string `config:"secret_key"`
}

func (s S3Config) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"BufferBucket": z.String().
			Required(z.Message("core.storage.s3.buffer_bucket is required")),
		"Endpoint": z.String().
			Required(z.Message("core.storage.s3.endpoint is required")),
		"Region": z.String().
			Required(z.Message("core.storage.s3.region is required")),
		"AccessKey": z.String().
			Required(z.Message("core.storage.s3.access_key is required")),
		"SecretKey": z.String().
			Required(z.Message("core.storage.s3.secret_key is required")),
	})
}

func (s S3Config) Defaults() map[string]any {
	return map[string]any{
		"BufferBucket": "",
		"Endpoint":     "",
		"Region":       "",
		"AccessKey":    "",
		"SecretKey":    "",
	}
}

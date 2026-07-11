package config

import (
	"encoding/hex"
	"fmt"

	z "github.com/Oudwins/zog"
	"go.lumeweb.com/configmanager"
)

var (
	_ configmanager.ConfigSchemaProvider = (*SiaConfig)(nil)
	_ Defaults                           = (*SiaConfig)(nil)
)

// S3StagingConfig configures the S3 staging backend for small object packing.
// When all fields are empty, the staging backend falls back to the main S3
// config (Core.Storage.S3) and uses BufferBucket.
type S3StagingConfig struct {
	Bucket    string `config:"bucket"`
	Endpoint  string `config:"endpoint"`
	Region    string `config:"region"`
	AccessKey string `config:"access_key"`
	SecretKey string `config:"secret_key"`
}

func (s S3StagingConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{})
}

func (s S3StagingConfig) Defaults() map[string]any {
	return map[string]any{
		"Bucket":    "",
		"Endpoint":  "",
		"Region":    "",
		"AccessKey": "",
		"SecretKey": "",
	}
}

type SiaConfig struct {
	AppKey        string          `config:"app_key"`
	URL           string          `config:"url"`
	Cluster       bool            `config:"cluster"`
	StagingType   string          `config:"staging_type"`
	DataShards    uint8           `config:"data_shards"`
	ParityShards  uint8           `config:"parity_shards"`
	S3Staging     S3StagingConfig `config:"s3_staging"`
}

func (s SiaConfig) Schema() z.ZogSchema {
	return z.Struct(z.Shape{
		"AppKey":     z.String(),
		"URL":        z.String(),
		"Cluster":    z.Bool().Default(false),
		"StagingType": z.String().Default("s3"),
		"DataShards":   z.UintLike[uint8]().Default(10),
		"ParityShards": z.UintLike[uint8]().Default(20),
	}).TestFunc(func(data any, ctx z.Ctx) bool {
		s, ok := data.(*SiaConfig)
		if !ok {
			return true
		}

		if s.AppKey != "" {
			if _, err := hex.DecodeString(s.AppKey); err != nil {
				ctx.AddIssue(ctx.Issue().SetMessage("app_key must be a valid hex-encoded string"))
				return false
			}
		}

		if s.Cluster && s.URL == "" {
			ctx.AddIssue(ctx.Issue().SetMessage("core.storage.sia.url is required when cluster is enabled"))
			return false
		}

		switch s.StagingType {
		case "", "s3", "memory":
		default:
			ctx.AddIssue(ctx.Issue().SetMessage("staging_type must be one of: s3, memory"))
			return false
		}

		// Validate redundancy if either shard count is set.
		if s.DataShards > 0 || s.ParityShards > 0 {
			total := int(s.DataShards) + int(s.ParityShards)
			redundancy := float64(total) / float64(s.DataShards)
			switch {
			case s.DataShards == 0:
				ctx.AddIssue(ctx.Issue().SetMessage("data_shards cannot be zero when parity_shards is set"))
				return false
			case total == 0:
				ctx.AddIssue(ctx.Issue().SetMessage("data_shards + parity_shards cannot be zero"))
				return false
			case total > 255:
				ctx.AddIssue(ctx.Issue().SetMessage("data_shards + parity_shards cannot exceed 255"))
				return false
			case int(s.ParityShards) > 255:
				ctx.AddIssue(ctx.Issue().SetMessage("parity_shards cannot exceed 255"))
				return false
			case redundancy < 1.5:
				ctx.AddIssue(ctx.Issue().SetMessage(fmt.Sprintf("redundancy of %.2f is too low (minimum 1.5x)", redundancy)))
				return false
			case redundancy > 4:
				ctx.AddIssue(ctx.Issue().SetMessage(fmt.Sprintf("redundancy of %.2f is too high (maximum 4x)", redundancy)))
				return false
			}
		}

		return true
	})
}

func (s SiaConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{
		"AppKey":       "",
		"Cluster":      false,
		"URL":          "",
		"StagingType":  "s3",
		"DataShards":   uint8(10),
		"ParityShards": uint8(20),
		"S3Staging":    S3StagingConfig{},
	}
}

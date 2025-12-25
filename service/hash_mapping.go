package service

import (
	"context"
	"encoding/json"
	"fmt"

	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	hashMappingMetrics "go.lumeweb.com/portal/service/internal/hash_mapping"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var _ core.HashMappingService = (*HashMappingServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.HASH_MAPPING_SERVICE,
		Factory: NewHashMappingService,
		Metrics: hashMappingMetrics.GetCollectors(),
	})
}

type HashMappingServiceDefault struct {
	core.Service
}

func NewHashMappingService() (core.Service, []core.ContextBuilderOption, error) {
	svc := &HashMappingServiceDefault{}

	return svc, nil, nil
}

func (h *HashMappingServiceDefault) ID() string {
	return core.HASH_MAPPING_SERVICE
}

func (h *HashMappingServiceDefault) StoreMapping(ctx context.Context, sourceHash, targetHash core.StorageHash, protocol string, metadata map[string]interface{}) error {
	return core.MetricTrack(
		hashMappingMetrics.MappingDuration.WithLabelValues(hashMappingMetrics.LabelOpStore),
		hashMappingMetrics.MappingFailed.WithLabelValues(hashMappingMetrics.LabelOpStore),
		func() error {
			if sourceHash == nil || targetHash == nil {
				return fmt.Errorf("sourceHash and targetHash cannot be nil")
			}

			metaJSON, err := json.Marshal(metadata)
			if err != nil {
				return fmt.Errorf("failed to marshal meta %w", err)
			}

			mapping := &models.HashMapping{
				SourceHash: sourceHash.Multihash(),
				TargetHash: targetHash.Multihash(),
				Protocol:   protocol,
				Metadata:   datatypes.JSON(metaJSON),
			}

			err = db.RetryableComponentTransaction(h, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.WithContext(ctx).Create(mapping)
			})

			if err == nil {
				hashMappingMetrics.MappingsStored.WithLabelValues(hashMappingMetrics.LabelOpStore).Inc()
			}
			return err
		},
	)
}

func (h *HashMappingServiceDefault) GetMappings(ctx context.Context, sourceHash core.StorageHash, protocol ...string) ([]core.StorageHash, error) {
	result, err := core.MetricTrackResult(
		hashMappingMetrics.MappingDuration.WithLabelValues(hashMappingMetrics.LabelOpGet),
		hashMappingMetrics.MappingFailed.WithLabelValues(hashMappingMetrics.LabelOpGet),
		func() ([]core.StorageHash, error) {
			var mappings []models.HashMapping

			err := db.RetryableComponentTransaction(h, ctx, func(tx *gorm.DB) *gorm.DB {
				query := tx.Where("source_hash = ?", sourceHash.Multihash())
				if len(protocol) > 0 {
					if protocol[0] == "" {
						query = query.Where("protocol = ?", "")
					} else {
						query = query.Where("protocol = ?", protocol[0])
					}
				}
				return query.Find(&mappings)
			})

			if err != nil {
				return nil, fmt.Errorf("failed to get mappings: %w", err)
			}

			hashes := make([]core.StorageHash, len(mappings))
			for i, mapping := range mappings {
				decode, _ := mh.Decode(mapping.TargetHash)
				if decode != nil {
					hashes[i] = core.NewStorageHash(decode.Digest, decode.Code, 0, nil)
				}
			}

			return hashes, nil
		},
	)

	if err == nil {
		hashMappingMetrics.MappingsQueried.WithLabelValues(hashMappingMetrics.LabelOpGet).Inc()
	}
	return result, err
}

func (h *HashMappingServiceDefault) GetReverseMappings(ctx context.Context, targetHash core.StorageHash, protocol ...string) ([]core.StorageHash, error) {
	result, err := core.MetricTrackResult(
		hashMappingMetrics.MappingDuration.WithLabelValues(hashMappingMetrics.LabelOpGetReverse),
		hashMappingMetrics.MappingFailed.WithLabelValues(hashMappingMetrics.LabelOpGetReverse),
		func() ([]core.StorageHash, error) {
			var mappings []models.HashMapping

			err := db.RetryableComponentTransaction(h, ctx, func(tx *gorm.DB) *gorm.DB {
				query := tx.Where("target_hash = ?", targetHash.Multihash())
				if len(protocol) > 0 && protocol[0] != "" {
					query = query.Where("protocol = ?", protocol[0])
				}
				return query.Find(&mappings)
			})

			if err != nil {
				return nil, fmt.Errorf("failed to get reverse mappings: %w", err)
			}

			hashes := make([]core.StorageHash, len(mappings))
			for i, mapping := range mappings {
				decode, _ := mh.Decode(mapping.SourceHash)
				if decode != nil {
					hashes[i] = core.NewStorageHash(decode.Digest, decode.Code, 0, nil)
				}
			}

			return hashes, nil
		},
	)

	if err == nil {
		hashMappingMetrics.MappingsQueried.WithLabelValues(hashMappingMetrics.LabelOpGetReverse).Inc()
	}
	return result, err
}

func (h *HashMappingServiceDefault) DeleteMappings(ctx context.Context, hash core.StorageHash) error {
	return core.MetricTrack(
		hashMappingMetrics.MappingDuration.WithLabelValues(hashMappingMetrics.LabelOpDelete),
		hashMappingMetrics.MappingFailed.WithLabelValues(hashMappingMetrics.LabelOpDelete),
		func() error {
			err := db.RetryableComponentTransaction(h, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Where("source_hash = ? OR target_hash = ?", hash.Multihash(), hash.Multihash()).
					Delete(&models.HashMapping{})
			})

			if err != nil {
				h.Logger().Error("Failed to delete hash mappings",
					zap.Error(err),
					zap.String("hash", hash.Multihash().String()))
				return fmt.Errorf("failed to delete mappings: %w", err)
			}

			hashMappingMetrics.MappingsDeleted.WithLabelValues(hashMappingMetrics.LabelOpDelete).Inc()
			return nil
		},
	)
}

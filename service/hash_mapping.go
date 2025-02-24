package service

import (
	"context"
	"encoding/json"
	"fmt"
	mh "github.com/multiformats/go-multihash"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

var _ core.HashMappingService = (*HashMappingServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.HASH_MAPPING_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewHashMappingService()
		},
	})
}

type HashMappingServiceDefault struct {
	ctx    core.Context
	db     *gorm.DB
	logger *core.Logger
}

func NewHashMappingService() (*HashMappingServiceDefault, []core.ContextBuilderOption, error) {
	svc := &HashMappingServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			svc.ctx = ctx
			svc.db = ctx.DB()
			svc.logger = ctx.ServiceLogger(svc)
			return nil
		}),
	)

	return svc, opts, nil
}

func (h *HashMappingServiceDefault) ID() string {
	return core.HASH_MAPPING_SERVICE
}

func (h *HashMappingServiceDefault) StoreMapping(ctx context.Context, sourceHash, targetHash core.StorageHash, protocol string, metadata map[string]interface{}) error {
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

	return db.RetryableTransaction(h.ctx, h.db, func(tx *gorm.DB) *gorm.DB {
		return tx.WithContext(ctx).Create(mapping)
	})
}

func (h *HashMappingServiceDefault) GetMappings(ctx context.Context, sourceHash core.StorageHash, protocol ...string) ([]core.StorageHash, error) {
	var mappings []models.HashMapping

	err := db.RetryableTransaction(h.ctx, h.db, func(tx *gorm.DB) *gorm.DB {
		query := tx.WithContext(ctx).Where("source_hash = ?", sourceHash.Multihash())
		if len(protocol) > 0 && protocol[0] != "" {
			query = query.Where("protocol = ?", protocol[0])
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
}

func (h *HashMappingServiceDefault) GetReverseMappings(ctx context.Context, targetHash core.StorageHash, protocol ...string) ([]core.StorageHash, error) {
	var mappings []models.HashMapping

	err := db.RetryableTransaction(h.ctx, h.db, func(tx *gorm.DB) *gorm.DB {
		query := tx.WithContext(ctx).Where("target_hash = ?", targetHash.Multihash())
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
}

func (h *HashMappingServiceDefault) DeleteMappings(ctx context.Context, hash core.StorageHash) error {
	err := db.RetryableTransaction(h.ctx, h.db, func(tx *gorm.DB) *gorm.DB {
		return tx.WithContext(ctx).Where("source_hash = ? OR target_hash = ?", hash.Multihash(), hash.Multihash()).
			Delete(&models.HashMapping{})
	})

	if err != nil {
		h.logger.Error("Failed to delete hash mappings",
			zap.Error(err),
			zap.String("hash", hash.Multihash().String()))
		return fmt.Errorf("failed to delete mappings: %w", err)
	}

	return nil
}

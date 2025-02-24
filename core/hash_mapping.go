package core

import (
	"context"
)

const HASH_MAPPING_SERVICE = "hash_mapping"

type HashMappingService interface {
	// Store a mapping between hashes - protocol required for documentation
	StoreMapping(ctx context.Context, sourceHash, targetHash StorageHash, protocol string, metadata map[string]interface{}) error

	// Look up target hash(es) for a source hash (protocol optional)
	GetMappings(ctx context.Context, sourceHash StorageHash, protocol ...string) ([]StorageHash, error)

	// Look up source hash(es) for a target hash (protocol optional)
	GetReverseMappings(ctx context.Context, targetHash StorageHash, protocol ...string) ([]StorageHash, error)

	// Delete mappings for a hash
	DeleteMappings(ctx context.Context, hash StorageHash) error

	Service
}

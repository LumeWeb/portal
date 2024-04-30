package sync

import (
	"github.com/LumeWeb/portal/sync/proto/gen/proto"
	"go.sia.tech/core/types"
	"go.sia.tech/renterd/object"
)

type FileMeta struct {
	Hash      []byte `json:"hash"`
	Multihash []byte `json:"multihash"`
	Proof     []byte `json:"proof"`
	Protocol  string `json:"protocol"`
	Key       object.EncryptionKey
	Size      uint64 `json:"size"`
	Slabs     []object.SlabSlice
	Aliases   []string `json:"aliases"`
}

func (fm *FileMeta) ToProtobuf() *proto.FileMeta {
	key, _ := fm.Key.MarshalBinary()
	slabSlices := make([]*proto.SlabSlice, 0, len(fm.Slabs))

	for _, slab := range fm.Slabs {
		key, _ := slab.Key.MarshalBinary()
		slabMeta := &proto.Slab{
			Health:    slab.Health,
			Key:       &proto.EncryptionKey{Entropy: key},
			MinShards: uint32(slab.MinShards),
		}

		shards := make([]*proto.Sector, 0, len(slab.Shards))

		for _, shard := range slab.Shards {
			contracts := make(map[string]*proto.FileContracts, len(shard.Contracts))
			for h, fcids := range shard.Contracts {
				fcidSet := &proto.FileContracts{
					Contracts: make([]*proto.FileContractID, 0, len(fcids)),
				}

				for _, fcid := range fcids {
					fhash := [32]byte(fcid)
					fcidSet.Contracts = append(fcidSet.Contracts, &proto.FileContractID{Id: fhash[:]})
				}
				contracts[h.String()] = fcidSet
			}

			shards = append(shards, &proto.Sector{
				ContractSet: contracts,
				LatestHost:  shard.LatestHost[:],
				Root:        shard.Root[:],
			})
		}

		slabMeta.Shards = shards

		slabSlices = append(slabSlices, &proto.SlabSlice{
			Slab:   slabMeta,
			Offset: slab.Offset,
			Length: slab.Length,
		})
	}

	return &proto.FileMeta{
		Hash:      fm.Hash,
		Proof:     fm.Proof,
		Multihash: fm.Multihash,
		Protocol:  fm.Protocol,
		Key:       &proto.EncryptionKey{Entropy: key},
		Size:      fm.Size,
		Slabs:     slabSlices,
		Aliases:   fm.Aliases,
	}
}

func (fm *FileMeta) keyRef() *object.EncryptionKey {
	return &fm.Key
}

func FileMetaFromProtobuf(fm *proto.FileMeta) (*FileMeta, error) {
	key := object.EncryptionKey{}
	err := key.UnmarshalBinary(fm.Key.Entropy)
	if err != nil {
		return nil, err
	}

	slabSlices := make([]object.SlabSlice, 0, len(fm.Slabs))

	for _, slab := range fm.Slabs {
		slabKey := object.EncryptionKey{}
		err := slabKey.UnmarshalBinary(slab.Slab.Key.Entropy)
		if err != nil {
			return nil, err
		}

		shards := make([]object.Sector, 0, len(slab.Slab.Shards))

		for _, sector := range slab.Slab.Shards {
			contracts := make(map[types.PublicKey][]types.FileContractID, len(sector.ContractSet))

			for h, fcidSet := range sector.ContractSet {
				fcids := make([]types.FileContractID, 0, len(fcidSet.Contracts))
				for _, fcid := range fcidSet.Contracts {
					fcids = append(fcids, types.FileContractID(fcid.Id[:]))
				}

				pubkey := types.PublicKey{}
				err := pubkey.UnmarshalText([]byte(h))
				if err != nil {
					return nil, err
				}
				contracts[pubkey] = fcids
			}

			root := make([]byte, 32)
			copy(root, sector.Root[:])

			shards = append(shards, object.Sector{
				Contracts:  contracts,
				LatestHost: types.PublicKey(sector.LatestHost),
				Root:       types.Hash256(root),
			})
		}

		slabSlices = append(slabSlices, object.SlabSlice{
			Offset: slab.Offset,
			Length: slab.Length,
			Slab: object.Slab{
				Health:    slab.Slab.Health,
				Key:       slabKey,
				MinShards: uint8(slab.Slab.MinShards),
				Shards:    shards,
			},
		})
	}

	return &FileMeta{
		Hash:      fm.Hash,
		Multihash: fm.Multihash,
		Proof:     fm.Proof,
		Protocol:  fm.Protocol,
		Key:       key,
		Size:      fm.Size,
		Slabs:     slabSlices,
		Aliases:   fm.Aliases,
	}, nil
}

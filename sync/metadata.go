package sync

import (
	"github.com/LumeWeb/portal/sync/proto/gen/proto"
	"go.sia.tech/renterd/object"
)

type FileMeta struct {
	Hash      []byte
	Multihash []byte
	Proof     []byte
	Protocol  string
	Key       object.EncryptionKey
	Size      uint64
	Slabs     []object.SlabSlice
	Aliases   []string
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

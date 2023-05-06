package cid

import (
	"encoding/binary"
	"github.com/multiformats/go-multibase"
)

func Encode(hash [32]byte, size uint64) (string, error) {
	sizeBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeBytes, size)

	prefixedHash := append([]byte{0x26, 0x1f}, hash[:]...)
	prefixedHash = append(prefixedHash, sizeBytes...)

	return multibase.Encode(multibase.Base58BTC, prefixedHash)
}

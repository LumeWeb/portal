package cid

import (
	"github.com/multiformats/go-multibase"
)

func EncodeHashSimple(hash [32]byte) (string, error) {
	prefixedHash := append([]byte{0x26, 0x1f}, hash[:]...)

	return multibase.Encode(multibase.Base58BTC, prefixedHash)
}

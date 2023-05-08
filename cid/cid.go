package cid

import (
	"bytes"
	"encoding/binary"
	"errors"
	"github.com/multiformats/go-multibase"
)

var MAGIC_BYTES = []byte{0x26, 0x1f}

type CID struct {
	Hash [32]byte
	Size uint64
}

func Encode(hash [32]byte, size uint64) (string, error) {
	sizeBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(sizeBytes, size)

	prefixedHash := append(MAGIC_BYTES, hash[:]...)
	prefixedHash = append(prefixedHash, sizeBytes...)

	return multibase.Encode(multibase.Base58BTC, prefixedHash)
}

func Valid(cid string) (bool, error) {
	_, err := maybeDecode(cid)
	if err != nil {
		return false, err
	}

	return true, nil

}

func Decode(cid string) (*CID, error) {
	data, err := maybeDecode(cid)
	if err != nil {
		return &CID{}, err
	}

	data = data[len(MAGIC_BYTES):]
	var hash [32]byte
	copy(hash[:], data[:])
	size := binary.LittleEndian.Uint64(data[32:])

	return &CID{Hash: hash, Size: size}, nil
}

func maybeDecode(cid string) ([]byte, error) {
	_, data, err := multibase.Decode(cid)
	if err != nil {
		return nil, err
	}

	if bytes.Compare(data[0:len(MAGIC_BYTES)], MAGIC_BYTES) != 0 {
		return nil, errors.New("CID magic bytes missing or invalid")
	}

	size := binary.LittleEndian.Uint64(data[len(MAGIC_BYTES)+32:])

	if size == 0 {
		return nil, errors.New("missing or empty size")
	}

	return data, nil
}

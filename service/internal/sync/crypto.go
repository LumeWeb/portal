package sync

import (
	"encoding/hex"
	"fmt"
	"golang.org/x/crypto/blake2b"
)

func namespace(name string, count interface{}) [][]byte {
	var ids []int
	switch v := count.(type) {
	case int:
		ids = make([]int, v)
		for i := 0; i < v; i++ {
			ids[i] = i
		}
	case []int:
		ids = v
	default:
		panic("Invalid count type")
	}

	buf := make([]byte, 32*len(ids))
	list := make([][]byte, len(ids))

	ns := make([]byte, 33)
	cryptoGenericHash(ns[:32], []byte(name), nil)

	for i := 0; i < len(list); i++ {
		list[i] = buf[32*i : 32*i+32]
		ns[32] = byte(ids[i])
		cryptoGenericHash(list[i], ns, nil)
	}

	return list
}

func manifestHash(manifest *manifest) []byte {
	if manifest.Version == 0 {
		return manifestHashv0(manifest)
	}

	buffer := make([]byte, 102)
	offset := 0

	// Write the manifest prefix
	copy(buffer[offset:32], MANIFEST)
	offset += 32

	// Write the version (assuming version is always 1)
	buffer[offset] = 1
	offset++

	flags := 0
	if manifest.AllowPatch {
		flags |= 1
	}
	if manifest.Prologue != nil {
		flags |= 2
	}

	// Write the flags
	buffer[offset] = byte(flags)
	offset++

	// Write the hash type (assuming hash type is always blake2b)
	buffer[offset] = 0
	offset++

	// Write the quorum (assuming quorum is always less than or equal to 0xfc)
	buffer[offset] = byte(manifest.Quorum)
	offset++

	buffer[offset] = byte(len(manifest.Signers))
	offset++
	// Write the signers (assuming signers is an array of Signer structs)
	for _, signer := range manifest.Signers {
		buffer[offset+1] = 0
		offset++
		// Write the namespace (assuming namespace is a 32-byte array)
		copy(buffer[offset:offset+32], signer.Namespace[:])
		offset += 32

		// Write the public key (assuming public key is a 32-byte array)
		copy(buffer[offset:offset+32], signer.PublicKey[:])
		offset += 32
	}

	fmt.Println(hex.EncodeToString(buffer))

	return hash(buffer, nil)
}

func manifestHashv0(manifest *manifest) []byte {
	buffer := make([]byte, 100)
	offset := 0

	// Write the manifest prefix
	copy(buffer[offset:32], MANIFEST)
	offset += 32

	// Write the version (assuming version is always 1)
	buffer[offset] = 0
	offset++

	// Write the hash type (assuming hash type is always blake2b)
	buffer[offset] = 0
	offset++

	// Write the quorum (assuming quorum is always less than or equal to 0xfc)
	buffer[offset] = byte(manifest.Quorum)
	offset++
	// Write the signers (assuming signers is an array of Signer structs)
	for _, signer := range manifest.Signers {
		buffer[offset+1] = 0
		offset++
		// Write the namespace (assuming namespace is a 32-byte array)
		copy(buffer[offset:offset+32], signer.Namespace[:])
		offset += 32

		// Write the public key (assuming public key is a 32-byte array)
		copy(buffer[offset:offset+32], signer.PublicKey[:])
		offset += 32
	}

	return hash(buffer, nil)
}

func hash(data interface{}, out []byte) []byte {
	if out == nil {
		out = make([]byte, 32)
	}

	var dataSlice [][]byte
	switch v := data.(type) {
	case []byte:
		dataSlice = [][]byte{v}
	case [][]byte:
		dataSlice = v
	default:
		panic("invalid data type")
	}

	cryptoGenericHashBatch(out, dataSlice, nil)

	return out
}

func cryptoGenericHashBatch(output []byte, inputArray [][]byte, key []byte) {
	hashSize := len(output)
	ctx, _ := blake2b.New(hashSize, key)

	for _, input := range inputArray {
		ctx.Write(input)
	}

	ctx.Sum(output[:0])
}
func cryptoGenericHash(output []byte, input []byte, key []byte) {
	hashSize := len(output)

	hasher, _ := blake2b.New(hashSize, key)
	hasher.Write(input)
	hasher.Sum(output[:0])
}

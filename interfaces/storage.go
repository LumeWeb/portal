package interfaces

import "io"

type StorageService interface {
	Init()
	PutFile(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error)
	FileExists(hash []byte) bool
	GetHash(file io.ReadSeeker) ([]byte, error)
}

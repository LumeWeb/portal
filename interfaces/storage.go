package interfaces

import "io"

type StorageService interface {
	Init()
	PutFile(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error)
}

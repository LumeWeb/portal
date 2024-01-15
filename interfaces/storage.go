package interfaces

import "io"

type StorageService interface {
	PutFile(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error)
}

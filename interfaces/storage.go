package interfaces

import "io"

type StorageService interface {
	Init()
	PutFile(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error)
	CIDExists(cid interface {
		ToString() (string, error)
	}) bool
}

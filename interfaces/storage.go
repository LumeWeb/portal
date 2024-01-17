package interfaces

import (
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"io"
)

type StorageService interface {
	Init()
	PutFile(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error)
	FileExists(hash []byte) (bool, models.Upload)
	GetHash(file io.ReadSeeker) ([]byte, error)
	CreateUpload(hash []byte, uploaderID uint, uploaderIP string, size uint64, protocol string) (*models.Upload, error)
}

package interfaces

import (
	"git.lumeweb.com/LumeWeb/portal/db/models"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"io"
)

type TusPreUploadCreateCallback func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error)
type TusPreFinishResponseCallback func(hook tusd.HookEvent) (tusd.HTTPResponse, error)

type StorageService interface {
	Portal() Portal
	PutFileSmall(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error)
	PutFile(file io.Reader, bucket string, hash []byte) error
	BuildUploadBufferTus(basePath string, preUploadCb TusPreUploadCreateCallback, preFinishCb TusPreFinishResponseCallback) (*tusd.Handler, tusd.DataStore, error)
	FileExists(hash []byte) (bool, models.Upload)
	GetHashSmall(file io.ReadSeeker) ([]byte, error)
	GetHash(file io.Reader) ([]byte, error)
	CreateUpload(hash []byte, uploaderID uint, uploaderIP string, size uint64, protocol string) (*models.Upload, error)
	TusUploadExists(hash []byte) (bool, models.Upload)
	CreateTusUpload(hash []byte, uploadID string, uploaderID uint, uploaderIP string, protocol string) (*models.TusUpload, error)
	TusUploadProgress(uploadID string) error
	DeleteTusUpload(uploadID string) error
	ScheduleTusUpload(uploadID string, attempt int) error
	Tus() *tusd.Handler
	Service
}

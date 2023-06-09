package files

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/bao"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/shared"
	"git.lumeweb.com/LumeWeb/portal/tusstore"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/viper"
	_ "github.com/tus/tusd/pkg/handler"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io"
	"strings"
)

const (
	STATUS_UPLOADED  = iota
	STATUS_UPLOADING = iota
	STATUS_NOT_FOUND = iota
)

var (
	ErrAlreadyExists          = errors.New("Upload already exists")
	ErrFailedFetchObject      = errors.New("Failed fetching object")
	ErrFailedFetchObjectProof = errors.New("Failed fetching object proof")
	ErrFailedFetchTusObject   = errors.New("Failed fetching tus object")
	ErrFailedHashFile         = errors.New("Failed to hash file")
	ErrFailedQueryTusUpload   = errors.New("Failed to query tus uploads")
	ErrFailedQueryUpload      = errors.New("Failed to query uploads table")
	ErrFailedSaveUpload       = errors.New("Failed saving upload to db")
	ErrFailedUpload           = errors.New("Failed uploading object")
	ErrFailedUploadProof      = errors.New("Failed uploading object proof")
	ErrFileExistsOutOfSync    = errors.New("File already exists in network, but missing in database")
	ErrFileHashMismatch       = errors.New("File hash does not match provided file hash")
	ErrInvalidFile            = errors.New("Invalid file")
)

var client *resty.Client

func Init() {
	client = resty.New()
	client.SetBaseURL("http://localhost:9980/api")
	client.SetBasicAuth("", viper.GetString("renterd-api-password"))
	client.SetDisableWarn(true)
}

func Upload(r io.ReadSeeker, size int64, hash []byte) (model.Upload, error) {
	var upload model.Upload

	tree, hashBytes, err := bao.ComputeTree(r, size)

	if err != nil {
		logger.Get().Error(ErrFailedHashFile.Error(), zap.Error(err))
		return upload, ErrFailedHashFile
	}

	if hash != nil {
		if bytes.Compare(hashBytes[:], hash) != 0 {
			logger.Get().Error(ErrFileHashMismatch.Error())
			return upload, ErrFileHashMismatch
		}
	}

	hashHex := hex.EncodeToString(hashBytes[:])

	_, err = r.Seek(0, io.SeekStart)

	if err != nil {
		return upload, err
	}

	result := db.Get().Where(&model.Upload{Hash: hashHex}).First(&upload)
	if (result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound)) || result.RowsAffected > 0 {
		err := result.Row().Scan(&upload)
		if err != nil {
			logger.Get().Error(ErrFailedQueryUpload.Error(), zap.Error(err))
			return upload, ErrFailedQueryUpload
		}

		if result.RowsAffected > 0 && upload.ID > 0 {
			logger.Get().Info(ErrAlreadyExists.Error())
			return upload, nil
		}
	}

	objectExistsResult, err := client.R().Get(getBusObjectUrl(hashHex))

	if err != nil {
		logger.Get().Error(ErrFailedQueryUpload.Error(), zap.Error(err))
		return upload, ErrFailedQueryUpload
	}

	objectStatusCode := objectExistsResult.StatusCode()

	if objectStatusCode == 500 {
		bodyErr := objectExistsResult.String()
		if !strings.Contains(bodyErr, "no slabs found") {
			logger.Get().Error(ErrFailedFetchObject.Error(), zap.String("error", objectExistsResult.String()))
			return upload, ErrFailedFetchObject
		}

		objectStatusCode = 404
	}

	proofExistsResult, err := client.R().Get(getBusProofUrl(hashHex))

	if err != nil {
		logger.Get().Error(ErrFailedFetchObjectProof.Error(), zap.Error(err))
		return upload, ErrFailedFetchObjectProof
	}

	proofStatusCode := proofExistsResult.StatusCode()

	if proofStatusCode == 500 {
		bodyErr := proofExistsResult.String()
		if !strings.Contains(bodyErr, "no slabs found") {
			logger.Get().Error(ErrFailedFetchObjectProof.Error(), zap.String("error", proofExistsResult.String()))
			return upload, ErrFailedFetchObjectProof
		}

		objectStatusCode = 404
	}

	if objectStatusCode != 404 && proofStatusCode != 404 {
		logger.Get().Error(ErrFileExistsOutOfSync.Error(), zap.String("hash", hashHex))
		return upload, ErrFileExistsOutOfSync
	}

	ret, err := client.R().SetBody(r).Put(getWorkerObjectUrl(hashHex))
	if ret.StatusCode() != 200 {
		logger.Get().Error(ErrFailedUpload.Error(), zap.String("error", ret.String()))
		return upload, ErrFailedUpload
	}

	ret, err = client.R().SetBody(tree).Put(getWorkerProofUrl(hashHex))
	if ret.StatusCode() != 200 {
		logger.Get().Error(ErrFailedUploadProof.Error(), zap.String("error", ret.String()))
		return upload, ErrFailedUpload
	}

	upload = model.Upload{
		Hash: hashHex,
	}

	if err = db.Get().Create(&upload).Error; err != nil {
		logger.Get().Error(ErrFailedSaveUpload.Error(), zap.Error(err))
		return upload, ErrFailedSaveUpload
	}

	return upload, nil
}
func Download(hash string) (io.Reader, error) {
	uploadItem := db.Get().Table("uploads").Where(&model.Upload{Hash: hash}).Row()
	tusItem := db.Get().Table("tus").Where(&model.Tus{Hash: hash}).Row()

	if uploadItem.Err() == nil {
		fetch, err := client.R().SetDoNotParseResponse(true).Get(fmt.Sprintf("/worker/objects/%s", hash))
		if err != nil {
			logger.Get().Error(ErrFailedFetchObject.Error(), zap.Error(err))
			return nil, ErrFailedFetchObject
		}

		return fetch.RawBody(), nil
	} else if tusItem.Err() == nil {
		var tusData model.Tus
		err := tusItem.Scan(&tusData)
		if err != nil {
			logger.Get().Error(ErrFailedQueryUpload.Error(), zap.Error(err))
			return nil, ErrFailedQueryUpload
		}

		upload, err := getStore().GetUpload(context.Background(), tusData.UploadID)
		if err != nil {
			logger.Get().Error(ErrFailedQueryTusUpload.Error(), zap.Error(err))
			return nil, ErrFailedQueryTusUpload
		}

		reader, err := upload.GetReader(context.Background())
		if err != nil {
			logger.Get().Error(ErrFailedFetchTusObject.Error(), zap.Error(err))
			return nil, ErrFailedFetchTusObject
		}

		return reader, nil
	} else {
		logger.Get().Error(ErrInvalidFile.Error(), zap.String("hash", hash))
		return nil, ErrInvalidFile
	}
}

func Status(hash string) int {
	var count int64

	uploadItem := db.Get().Table("uploads").Where(&model.Upload{Hash: hash}).Count(&count)

	if uploadItem.Error != nil && !errors.Is(uploadItem.Error, gorm.ErrRecordNotFound) {
		logger.Get().Error(ErrFailedQueryUpload.Error(), zap.Error(uploadItem.Error))
	}

	if count > 0 {
		return STATUS_UPLOADED
	}

	tusItem := db.Get().Table("tus").Where(&model.Tus{Hash: hash}).Count(&count)

	if tusItem.Error != nil && !errors.Is(tusItem.Error, gorm.ErrRecordNotFound) {
		logger.Get().Error(ErrFailedQueryUpload.Error(), zap.Error(tusItem.Error))
	}

	if count > 0 {
		return STATUS_UPLOADING
	}

	return STATUS_NOT_FOUND
}

func objectUrlBuilder(hash string, bus bool, proof bool) string {
	path := []string{}
	if bus {
		path = append(path, "bus")
	} else {
		path = append(path, "worker")
	}

	path = append(path, "objects")

	name := "%s"

	if proof {
		name = name + ".obao"
	}

	path = append(path, name)

	return fmt.Sprintf(strings.Join(path, "/"), hash)
}

func getBusObjectUrl(hash string) string {
	return objectUrlBuilder(hash, true, false)
}
func getWorkerObjectUrl(hash string) string {
	return objectUrlBuilder(hash, false, false)
}
func getWorkerProofUrl(hash string) string {
	return objectUrlBuilder(hash, false, true)
}
func getBusProofUrl(hash string) string {
	return objectUrlBuilder(hash, true, true)
}

func getStore() *tusstore.DbFileStore {
	ret := shared.GetTusStore()
	return (*ret).(*tusstore.DbFileStore)
}

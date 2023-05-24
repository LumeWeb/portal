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
	"git.lumeweb.com/LumeWeb/portal/renterd"
	"git.lumeweb.com/LumeWeb/portal/shared"
	"git.lumeweb.com/LumeWeb/portal/tusstore"
	"github.com/go-resty/resty/v2"
	_ "github.com/tus/tusd/pkg/handler"
	"go.uber.org/zap"
	"io"
	"strings"
)

var client *resty.Client

func Init() {
	client = resty.New()
	client.SetBaseURL(renterd.GetApiAddr() + "/api")
	client.SetBasicAuth("", renterd.GetAPIPassword())
	client.SetDisableWarn(true)
}

func Upload(r io.ReadSeeker, size int64, hash []byte) (model.Upload, error) {
	var upload model.Upload

	tree, hashBytes, err := bao.ComputeTree(r, size)

	if err != nil {
		logger.Get().Error("Failed to hash file", zap.Error(err))
		return upload, err
	}

	if hash != nil {
		if bytes.Compare(hashBytes[:], hash) != 0 {
			logger.Get().Error("File hash does not match provided file hash")
			return upload, err
		}
	}

	hashHex := hex.EncodeToString(hashBytes[:])

	_, err = r.Seek(0, io.SeekStart)

	if err != nil {
		return upload, err
	}

	result := db.Get().Where(&model.Upload{Hash: hashHex}).First(&upload)
	if (result.Error != nil && result.Error.Error() != "record not found") || result.RowsAffected > 0 {
		err := result.Row().Scan(&upload)
		if err != nil {
			logger.Get().Error("Failed to query uploads table", zap.Error(err))
			return upload, err
		}

		if result.RowsAffected > 0 && upload.ID > 0 {
			logger.Get().Info("Upload already exists")
			return upload, nil
		}
	}

	objectExistsResult, err := client.R().Get(getBusObjectUrl(hashHex))

	if err != nil {
		logger.Get().Error("Failed query object", zap.Error(err))
		return upload, err
	}

	objectStatusCode := objectExistsResult.StatusCode()

	if objectStatusCode == 500 {
		bodyErr := objectExistsResult.String()
		if !strings.Contains(bodyErr, "no slabs found") {
			logger.Get().Error("Failed fetching object", zap.String("error", objectExistsResult.String()))
			return upload, errors.New(fmt.Sprintf("error fetching file: %s", objectExistsResult.String()))
		}

		objectStatusCode = 404
	}

	proofExistsResult, err := client.R().Get(getBusProofUrl(hashHex))

	if err != nil {
		logger.Get().Error("Failed query object proof", zap.Error(err))
		return upload, err
	}

	proofStatusCode := proofExistsResult.StatusCode()

	if proofStatusCode == 500 {
		bodyErr := proofExistsResult.String()
		if !strings.Contains(bodyErr, "no slabs found") {
			logger.Get().Error("Failed fetching object proof", zap.String("error", proofExistsResult.String()))
			return upload, errors.New(fmt.Sprintf("error fetching file proof: %s", proofExistsResult.String()))
		}

		objectStatusCode = 404
	}

	if objectStatusCode != 404 && proofStatusCode != 404 {
		msg := "file already exists in network, but missing in database"
		logger.Get().Error(msg)
		return upload, errors.New(msg)
	}

	ret, err := client.R().SetBody(r).Put(getWorkerObjectUrl(hashHex))
	if ret.StatusCode() != 200 {
		logger.Get().Error("Failed uploading object", zap.String("error", ret.String()))
		err = errors.New(ret.String())
		return upload, err
	}

	ret, err = client.R().SetBody(tree).Put(getWorkerProofUrl(hashHex))
	if ret.StatusCode() != 200 {
		logger.Get().Error("Failed uploading proof", zap.String("error", ret.String()))
		err = errors.New(ret.String())
		return upload, err
	}

	upload = model.Upload{
		Hash: hashHex,
	}

	if err = db.Get().Create(&upload).Error; err != nil {
		logger.Get().Error("Failed adding upload to db", zap.Error(err))
		return upload, err
	}

	return upload, nil
}
func Download(hash string) (io.Reader, error) {
	uploadItem := db.Get().Table("uploads").Where(&model.Upload{Hash: hash}).Row()
	tusItem := db.Get().Table("tus").Where(&model.Tus{Hash: hash}).Row()

	if uploadItem.Err() == nil {
		fetch, err := client.R().SetDoNotParseResponse(true).Get(fmt.Sprintf("/worker/objects/%s", hash))
		if err != nil {
			logger.Get().Error("Failed downloading object", zap.Error(err))
			return nil, err
		}

		return fetch.RawBody(), nil
	} else if tusItem.Err() == nil {
		var tusData model.Tus
		err := tusItem.Scan(&tusData)
		if err != nil {
			logger.Get().Error("Failed querying upload from db", zap.Error(err))
			return nil, err
		}

		upload, err := getStore().GetUpload(context.Background(), tusData.UploadID)
		if err != nil {
			logger.Get().Error("Failed querying tus upload", zap.Error(err))
			return nil, err
		}

		reader, err := upload.GetReader(context.Background())
		if err != nil {
			logger.Get().Error("Failed reading tus upload", zap.Error(err))
			return nil, err
		}

		return reader, nil
	} else {
		logger.Get().Error("invalid file")
		return nil, errors.New("invalid file")
	}
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

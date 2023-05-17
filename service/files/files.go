package files

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/bao"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/renterd"
	"git.lumeweb.com/LumeWeb/portal/shared"
	"github.com/go-resty/resty/v2"
	"io"
)

var client *resty.Client

func Init() {
	client = resty.New()
	client.SetBaseURL(renterd.GetApiAddr() + "/api")
	client.SetBasicAuth("", renterd.GetAPIPassword())
	client.SetDisableWarn(true)
}

func Upload(r io.ReadSeeker, size int64) (model.Upload, error) {
	var upload model.Upload

	tree, hashBytes, err := bao.ComputeTree(r, size)

	if err != nil {
		return upload, err
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
			return upload, err
		}
	}

	objectExistsResult, err := client.R().Get(fmt.Sprintf("/worker/objects/%s", hashHex))

	if err != nil {
		return upload, err
	}

	if objectExistsResult.StatusCode() != 404 {
		return upload, errors.New("file already exists in network, but missing in database")
	}

	if err != nil {
		return upload, err
	}

	ret, err := client.R().SetBody(r).Put(fmt.Sprintf("/worker/objects/%s", hashHex))
	if ret.StatusCode() != 200 {
		err = errors.New(string(ret.Body()))
		return upload, err
	}

	ret, err = client.R().SetBody(tree).Put(fmt.Sprintf("/worker/objects/%s.obao", hashHex))
	if ret.StatusCode() != 200 {
		err = errors.New(string(ret.Body()))
		return upload, err
	}

	upload = model.Upload{
		Hash: hashHex,
	}

	if err = db.Get().Create(&upload).Error; err != nil {
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
			return nil, err
		}

		return fetch.RawBody(), nil
	} else if tusItem.Err() == nil {
		var tusData model.Tus
		err := tusItem.Scan(&tusData)
		if err != nil {
			return nil, err
		}

		upload, err := shared.GetTusStore().GetUpload(context.Background(), tusData.Id)
		if err != nil {
			return nil, err
		}

		reader, err := upload.GetReader(context.Background())
		if err != nil {
			return nil, err
		}

		return reader, nil
	} else {
		return nil, errors.New("invalid file")
	}
}

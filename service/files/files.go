package files

import (
	"bufio"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/bao"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/renterd"
	"github.com/go-resty/resty/v2"
	"io"
	"lukechampine.com/blake3"
)

var client *resty.Client

func Init() {
	client = resty.New()
	client.SetBaseURL(renterd.GetApiAddr() + "/api")
	client.SetBasicAuth("", renterd.GetAPIPassword())
	client.SetDisableWarn(true)
}

func Upload(r io.ReadSeeker) (model.Upload, error) {
	var upload model.Upload

	hasher := blake3.New(32, nil)

	_, err := io.Copy(hasher, r)
	if err != nil {
		return upload, err
	}

	hashBytes := hasher.Sum(nil)

	hashHex := hex.EncodeToString(hashBytes[:])

	if err != nil {
		return upload, err
	}

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

	tree, err := bao.ComputeBaoTree(bufio.NewReader(r))

	if err != nil {
		return upload, err
	}

	_, err = r.Seek(0, io.SeekStart)
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
	result := db.Get().Table("uploads").Where(&model.Upload{Hash: hash}).Row()

	if result.Err() != nil {
		return nil, result.Err()
	}

	fetch, err := client.R().SetDoNotParseResponse(true).Get(fmt.Sprintf("/worker/objects/%s", hash))
	if err != nil {
		return nil, err
	}

	return fetch.RawBody(), nil
}

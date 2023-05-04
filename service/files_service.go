package service

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/bao"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/model"
	"git.lumeweb.com/LumeWeb/portal/renterd"
	"github.com/go-resty/resty/v2"
	"github.com/kataras/iris/v12"
	"io"
	"lukechampine.com/blake3"
)

type FilesService struct {
	Ctx iris.Context
}

var client *resty.Client

type UploadResponse struct {
	Cid string `json:"cid"`
}

func InitFiles() {
	client = resty.New()
	client.SetBaseURL(renterd.GetApiAddr() + "/api")
	client.SetBasicAuth("", renterd.GetAPIPassword())
}

func (f *FilesService) PostUpload() {
	ctx := f.Ctx

	file, _, err := f.Ctx.FormFile("file")
	if internalErrorCustom(ctx, err, errors.New("invalid file data")) {
		return
	}

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(file)

	if internalErrorCustom(ctx, err, errors.New("failed to read file data")) {
		return
	}

	hashBytes := blake3.Sum256(buf.Bytes())
	hashHex := hex.EncodeToString(hashBytes[:])
	fileCid, err := cid.EncodeHashSimple(hashBytes)

	if internalError(ctx, err) {
		return
	}

	var upload model.Upload
	result := db.Get().Where("hash = ?", hashHex).First(&upload)
	if (result.Error != nil && result.Error.Error() != "record not found") || result.RowsAffected > 0 {
		ctx.JSON(&UploadResponse{Cid: fileCid})
		return
	}

	_, err = file.Seek(0, io.SeekStart)
	if internalError(ctx, err) {
		return
	}

	tree, err := bao.ComputeBaoTree(file)
	if internalError(ctx, err) {
		return
	}

	objectExistsResult, err := client.R().Get(fmt.Sprintf("/worker/objects/%s", hashHex))

	if internalError(ctx, err) {
		return
	}

	if objectExistsResult.StatusCode() != 404 {
		ctx.JSON(&UploadResponse{Cid: fileCid})
		return
	}

	ret, err := client.R().SetBody(file).Put(fmt.Sprintf("/worker/objects/%s", hashHex))
	if internalError(ctx, err) {
		return
	}
	fmt.Println(ret)

	_, err = client.R().SetBody(tree).Put(fmt.Sprintf("/worker/objects/%s.obao", hashHex))
	if internalError(ctx, err) {
		return
	}

	upload = model.Upload{
		Hash: hashHex,
	}
	if err := db.Get().Create(&upload).Error; err != nil {
		if internalError(ctx, err) {
			return
		}
	}

	ctx.JSON(&UploadResponse{Cid: fileCid})
}

func internalErrorCustom(ctx iris.Context, err error, customError error) bool {
	if err != nil {
		if customError != nil {
			err = customError
		}
		ctx.StopWithError(iris.StatusInternalServerError, err)
		return true
	}

	return false
}
func internalError(ctx iris.Context, err error) bool {
	return internalErrorCustom(ctx, err, nil)
}

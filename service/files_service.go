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
	client.SetDisableWarn(true)
}

func (f *FilesService) PostUpload() {
	ctx := f.Ctx

	file, meta, err := f.Ctx.FormFile("file")
	if internalErrorCustom(ctx, err, errors.New("invalid file data")) {
		return
	}

	buf, err := io.ReadAll(file)
	if internalError(ctx, err) {
		return
	}

	if internalErrorCustom(ctx, err, errors.New("failed to read file data")) {
		return
	}

	hashBytes := blake3.Sum256(buf)
	hashHex := hex.EncodeToString(hashBytes[:])
	fileCid, err := cid.Encode(hashBytes, uint64(meta.Size))

	if internalError(ctx, err) {
		return
	}

	_, err = file.Seek(0, io.SeekStart)
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

	tree, err := bao.ComputeBaoTree(bytes.NewReader(buf))
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

	if internalError(ctx, err) {
		return
	}

	ret, err := client.R().SetBody(buf).Put(fmt.Sprintf("/worker/objects/%s", hashHex))
	if ret.StatusCode() != 200 {
		err = errors.New(string(ret.Body()))
	}
	if internalError(ctx, err) {
		return
	}

	ret, err = client.R().SetBody(tree).Put(fmt.Sprintf("/worker/objects/%s.obao", hashHex))
	if ret.StatusCode() != 200 {
		err = errors.New(string(ret.Body()))
	}
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

func (f *FilesService) GetDownload() {
	ctx := f.Ctx

	cidString := ctx.URLParam("cid")

	_, err := cid.Valid(cidString)
	if sendError(ctx, err, iris.StatusBadRequest) {
		return
	}

	cidObject, _ := cid.Decode(cidString)
	hashHex := hex.EncodeToString(cidObject.Hash[:])

	result := db.Get().Table("uploads").Where("hash = ?", hashHex).Row()

	if result.Err() != nil {
		sendError(ctx, result.Err(), iris.StatusNotFound)
		return
	}

	fetch, err := client.R().SetDoNotParseResponse(true).Get(fmt.Sprintf("/worker/objects/%s", hashHex))
	if err != nil {
		if fetch.StatusCode() == 404 {
			sendError(ctx, err, iris.StatusNotFound)
			return
		}
		internalError(ctx, err)
		return
	}

	ctx.Header("Transfer-Encoding", "chunked")

	if internalError(ctx, err) {
		return
	}

	err = ctx.StreamWriter(func(w io.Writer) error {
		_, err = io.Copy(w, fetch.RawBody())
		_ = fetch.RawBody().Close()
		return err
	})
	internalError(ctx, err)

}

func sendErrorCustom(ctx iris.Context, err error, customError error, irisError int) bool {
	if err != nil {
		if customError != nil {
			err = customError
		}
		ctx.StopWithError(irisError, err)
		return true
	}

	return false
}
func internalError(ctx iris.Context, err error) bool {
	return sendErrorCustom(ctx, err, nil, iris.StatusInternalServerError)
}
func internalErrorCustom(ctx iris.Context, err error, customError error) bool {
	return sendErrorCustom(ctx, err, customError, iris.StatusInternalServerError)
}
func sendError(ctx iris.Context, err error, irisError int) bool {
	return sendErrorCustom(ctx, err, nil, irisError)
}

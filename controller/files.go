package controller

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"github.com/kataras/iris/v12"
	"io"
)

type FilesController struct {
	Ctx iris.Context
}
type UploadResponse struct {
	Cid string `json:"cid"`
}

func (f *FilesController) PostUpload() {
	ctx := f.Ctx

	file, meta, err := f.Ctx.FormFile("file")
	if internalErrorCustom(ctx, err, errors.New("invalid file data")) {
		return
	}

	upload, err := files.Upload(file, meta.Size)

	if internalError(ctx, err) {
		return
	}

	cidString, err := cid.EncodeString(upload.Hash, uint64(meta.Size))

	if internalError(ctx, err) {
		return
	}

	_ = ctx.JSON(&UploadResponse{Cid: cidString})
}

func (f *FilesController) GetDownloadBy(cidString string) {
	ctx := f.Ctx

	_, err := cid.Valid(cidString)
	if sendError(ctx, err, iris.StatusBadRequest) {
		return
	}

	cidObject, _ := cid.Decode(cidString)
	hashHex := cidObject.StringHash()

	if internalError(ctx, err) {
		return
	}

	download, err := files.Download(hashHex)
	if internalError(ctx, err) {
		return
	}

	err = ctx.StreamWriter(func(w io.Writer) error {
		_, err = io.Copy(w, download)
		_ = download.(io.Closer).Close()
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

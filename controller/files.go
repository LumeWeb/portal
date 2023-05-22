package controller

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
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
		logger.Get().Debug("invalid file data", zap.Error(err))
		return
	}

	upload, err := files.Upload(file, meta.Size, nil)

	if internalError(ctx, err) {
		logger.Get().Debug("failed uploading file", zap.Error(err))
		return
	}

	cidString, err := cid.EncodeString(upload.Hash, uint64(meta.Size))

	if internalError(ctx, err) {
		logger.Get().Debug("failed creating cid", zap.Error(err))
		return
	}

	err = ctx.JSON(&UploadResponse{Cid: cidString})

	if err != nil {
		logger.Get().Error("failed to create response", zap.Error(err))
	}
}

func (f *FilesController) GetDownloadBy(cidString string) {
	ctx := f.Ctx

	_, err := cid.Valid(cidString)
	if sendError(ctx, err, iris.StatusBadRequest) {
		logger.Get().Debug("invalid cid", zap.Error(err))
		return
	}

	cidObject, _ := cid.Decode(cidString)
	hashHex := cidObject.StringHash()
	download, err := files.Download(hashHex)
	if internalError(ctx, err) {
		logger.Get().Debug("failed fetching file", zap.Error(err))
		return
	}

	err = ctx.StreamWriter(func(w io.Writer) error {
		_, err = io.Copy(w, download)
		_ = download.(io.Closer).Close()
		return err
	})
	if internalError(ctx, err) {
		logger.Get().Debug("failed streaming file", zap.Error(err))
	}
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

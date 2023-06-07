package controller

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/controller/response"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
	"io"
)

type FilesController struct {
	Ctx iris.Context
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

	err = ctx.JSON(&response.UploadResponse{Cid: cidString})

	if err != nil {
		logger.Get().Error("failed to create response", zap.Error(err))
	}
}

func (f *FilesController) GetDownloadBy(cidString string) {
	ctx := f.Ctx

	hashHex, valid := validateCid(cidString, true, ctx)

	if !valid {
		return
	}

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

func (f *FilesController) GetStatusBy(cidString string) {
	ctx := f.Ctx

	hashHex, valid := validateCid(cidString, false, ctx)

	if !valid {
		return
	}

	status := files.Status(hashHex)

	var statusCode string

	switch status {
	case files.STATUS_UPLOADED:
		statusCode = "uploaded"
		break
	case files.STATUS_UPLOADING:
		statusCode = "uploading"
		break
	case files.STATUS_NOT_FOUND:
		statusCode = "uploading"
		break
	}

	err := ctx.JSON(&response.StatusResponse{Status: statusCode})

	if err != nil {
		logger.Get().Error("failed to create response", zap.Error(err))
	}

}
func validateCid(cidString string, validateStatus bool, ctx iris.Context) (string, bool) {
	_, err := cid.Valid(cidString)
	if sendError(ctx, err, iris.StatusBadRequest) {
		logger.Get().Debug("invalid cid", zap.Error(err))
		return "", false
	}

	cidObject, _ := cid.Decode(cidString)
	hashHex := cidObject.StringHash()

	if validateStatus {
		status := files.Status(hashHex)

		if status == files.STATUS_NOT_FOUND {
			err := errors.New("cid not found")
			sendError(ctx, errors.New("cid not found"), iris.StatusNotFound)
			logger.Get().Debug("cid not found", zap.Error(err))
			return "", false
		}
	}

	return hashHex, true
}

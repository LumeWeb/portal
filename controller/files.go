package controller

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/cid"
	"git.lumeweb.com/LumeWeb/portal/controller/response"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/middleware"
	"git.lumeweb.com/LumeWeb/portal/service/files"
	"github.com/kataras/iris/v12"
	"go.uber.org/zap"
	"io"
)

var errStreamDone = errors.New("done")

type FilesController struct {
	Controller
}

func (f *FilesController) BeginRequest(ctx iris.Context) {
	middleware.VerifyJwt(ctx)
}
func (f *FilesController) EndRequest(ctx iris.Context) {
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

	err = files.Pin(upload.Hash, upload.AccountID)

	if internalError(ctx, err) {
		logger.Get().Debug("failed pinning file", zap.Error(err))
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

	err = passThroughStream(download, ctx)
	if err != errStreamDone && internalError(ctx, err) {
		logger.Get().Debug("failed streaming file", zap.Error(err))
	}
}

func (f *FilesController) GetProofBy(cidString string) {
	ctx := f.Ctx

	hashHex, valid := validateCid(cidString, true, ctx)

	if !valid {
		return
	}

	proof, err := files.DownloadProof(hashHex)
	if internalError(ctx, err) {
		logger.Get().Debug("failed fetching file proof", zap.Error(err))
		return
	}

	err = passThroughStream(proof, ctx)
	if internalError(ctx, err) {
		logger.Get().Debug("failed streaming file proof", zap.Error(err))
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
		statusCode = "not_found"
		break
	}

	f.respondJSON(&response.FileStatusResponse{Status: statusCode})

}

func (f *FilesController) PostPinBy(cidString string) {
	ctx := f.Ctx

	hashHex, valid := validateCid(cidString, true, ctx)

	if !valid {
		return
	}

	err := files.Pin(hashHex, getCurrentUserId(ctx))
	if internalError(ctx, err) {
		logger.Get().Error(err.Error())
		return
	}

	f.Ctx.StatusCode(iris.StatusCreated)
}

func (f *FilesController) GetUploadLimit() {
	f.respondJSON(&response.UploadLimit{Limit: f.Ctx.Application().ConfigurationReadOnly().GetPostMaxMemory()})
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

func passThroughStream(stream io.Reader, ctx iris.Context) error {
	closed := false

	err := ctx.StreamWriter(func(w io.Writer) error {
		if closed {
			return errStreamDone
		}

		count, err := io.CopyN(w, stream, 1024)
		if count == 0 || err == io.EOF {
			err = stream.(io.Closer).Close()
			if err != nil {
				logger.Get().Error("failed closing stream", zap.Error(err))
				return err
			}
			closed = true
			return nil
		}

		if err != nil {
			return err
		}

		return nil
	})

	if err == errStreamDone {
		err = nil
	}

	return err
}

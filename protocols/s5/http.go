package s5

import (
	"bytes"
	"errors"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	s5interface "git.lumeweb.com/LumeWeb/libs5-go/interfaces"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"go.sia.tech/jape"
	"go.uber.org/zap"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

var (
	_ s5interface.HTTPHandler = (*HttpHandlerImpl)(nil)
)

const (
	errMultiformParse = "Error parsing multipart form"
	errRetrievingFile = "Error retrieving the file"
	errReadFile       = "Error reading the file"
	errClosingStream  = "Error closing the stream"
	errUploadingFile  = "Error uploading the file"
)

var (
	errUploadingFileErr = errors.New(errUploadingFile)
)

type HttpHandlerImpl struct {
	portal interfaces.Portal
}

func NewHttpHandler(portal interfaces.Portal) *HttpHandlerImpl {
	return &HttpHandlerImpl{portal: portal}
}

func (h *HttpHandlerImpl) SmallFileUpload(jc *jape.Context) {
	buffer := bytes.NewBuffer(nil)

	r := jc.Request
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Parse the multipart form
		err := r.ParseMultipartForm(h.portal.Config().GetInt64("core.post-upload-limit"))

		if jc.Check(errMultiformParse, err) != nil {
			h.portal.Logger().Error(errMultiformParse, zap.Error(err))
			return
		}

		// Retrieve the file from the form data
		file, _, err := r.FormFile("file")
		if jc.Check(errRetrievingFile, err) != nil {
			h.portal.Logger().Error(errRetrievingFile, zap.Error(err))
			return
		}
		defer func(file multipart.File) {
			err := file.Close()
			if err != nil {
				h.portal.Logger().Error(errClosingStream, zap.Error(err))
			}
		}(file)

		// Copy file contents to buffer
		_, err = io.Copy(buffer, file)
		if jc.Check(errReadFile, err) != nil {
			h.portal.Logger().Error(errReadFile, zap.Error(err))
			return
		}
	} else {
		// For other content types, read the body into the buffer
		_, err := io.Copy(buffer, r.Body)

		if jc.Check(errReadFile, err) != nil {
			h.portal.Logger().Error(errReadFile, zap.Error(err))
			return
		}

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				h.portal.Logger().Error(errClosingStream, zap.Error(err))
			}
		}(r.Body)
	}

	hash, err := h.portal.Storage().PutFile(bytes.NewReader(buffer.Bytes()), "s5", false)

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errUploadingFile, zap.Error(err))
		return
	}

	cid, err := encoding.CIDFromHash(hash, uint64(len(buffer.Bytes())), types.CIDTypeRaw)

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errUploadingFile, zap.Error(err))
		return
	}

	cidStr, err := cid.ToString()

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errUploadingFile, zap.Error(err))
		return
	}

	jc.Encode(map[string]string{"hash": cidStr})
}

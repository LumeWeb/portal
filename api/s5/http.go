package s5

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"go.sia.tech/jape"
	"go.uber.org/zap"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

const (
	errMultiformParse           = "Error parsing multipart form"
	errRetrievingFile           = "Error retrieving the file"
	errReadFile                 = "Error reading the file"
	errClosingStream            = "Error closing the stream"
	errUploadingFile            = "Error uploading the file"
	errAccountGenerateChallenge = "Error generating challenge"
)

var (
	errUploadingFileErr            = errors.New(errUploadingFile)
	errAccountGenerateChallengeErr = errors.New(errAccountGenerateChallenge)
)

type HttpHandler struct {
	portal interfaces.Portal
}

func NewHttpHandler(portal interfaces.Portal) *HttpHandler {
	return &HttpHandler{portal: portal}
}

func (h *HttpHandler) SmallFileUpload(jc jape.Context) {
	var rs io.ReadSeeker
	var bufferSize int64

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

		rs = file
	} else {
		data, err := io.ReadAll(r.Body)
		if jc.Check(errReadFile, err) != nil {
			h.portal.Logger().Error(errReadFile, zap.Error(err))
			return
		}

		buffer := bytes.NewReader(data)
		bufferSize = int64(buffer.Len())
		rs = buffer

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				h.portal.Logger().Error(errClosingStream, zap.Error(err))
			}
		}(r.Body)
	}

	hash, err := h.portal.Storage().GetHash(rs)

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errUploadingFile, zap.Error(err))
		return
	}

	if exists, upload := h.portal.Storage().FileExists(hash); exists {
		cid, err := encoding.CIDFromHash(hash, upload.Size, types.CIDTypeRaw)
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
		return
	}

	hash, err = h.portal.Storage().PutFile(rs, "s5", false)

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errUploadingFile, zap.Error(err))
		return
	}

	cid, err := encoding.CIDFromHash(hash, uint64(bufferSize), types.CIDTypeRaw)

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

	tx := h.portal.Database().Create(&models.Upload{
		Hash:     hex.EncodeToString(hash),
		Size:     uint64(bufferSize),
		Protocol: "s5",
		UserID:   0,
	})

	if tx.Error != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errUploadingFile, zap.Error(err))
		return
	}

	jc.Encode(map[string]string{"hash": cidStr})
}

func (h *HttpHandler) AccountRegisterChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	challenge := make([]byte, 32)

	_, err := rand.Read(challenge)
	if err != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)

	if err != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	if len(decodedKey) != 32 {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	result := h.portal.Database().Create(&models.S5Challenge{
		Challenge: hex.EncodeToString(challenge),
	})

	if result.Error != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	jc.Encode(map[string]string{"challenge": base64.RawURLEncoding.EncodeToString(challenge)})
}

func (h *HttpHandler) AccountRegister(context jape.Context) {
	//TODO implement me
	panic("implement me")
}

func (h *HttpHandler) AccountLoginChallenge(context jape.Context) {
	//TODO implement me
	panic("implement me")
}

func (h *HttpHandler) AccountLogin(context jape.Context) {
	//TODO implement me
	panic("implement me")
}

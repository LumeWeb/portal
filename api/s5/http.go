package s5

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/metadata"
	libs5node "git.lumeweb.com/LumeWeb/libs5-go/node"
	libs5protocol "git.lumeweb.com/LumeWeb/libs5-go/protocol"
	libs5service "git.lumeweb.com/LumeWeb/libs5-go/service"
	libs5storage "git.lumeweb.com/LumeWeb/libs5-go/storage"
	libs5storageProvider "git.lumeweb.com/LumeWeb/libs5-go/storage/provider"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/protocols/s5"
	"git.lumeweb.com/LumeWeb/portal/storage"
	emailverifier "github.com/AfterShip/email-verifier"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/vmihailenco/msgpack/v5"
	"go.sia.tech/jape"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"nhooyr.io/websocket"
	"strconv"
	"strings"
	"time"
)

const (
	errMultiformParse           = "Error parsing multipart form"
	errRetrievingFile           = "Error retrieving the file"
	errReadFile                 = "Error reading the file"
	errClosingStream            = "Error closing the stream"
	errUploadingFile            = "Error uploading the file"
	errAccountGenerateChallenge = "Error generating challenge"
	errAccountRegister          = "Error registering account"
	errAccountLogin             = "Error logging in account"
	errFailedToGetPins          = "Failed to get pins"
	errFailedToDelPin           = "Failed to delete pin"
	errFailedToAddPin           = "Failed to add pin"
	errorNotMultiform           = "Not a multipart form"
	errFetchingUrls             = "Error fetching urls"
	errDownloadingFile          = "Error downloading file"
)

var (
	errUploadingFileErr            = errors.New(errUploadingFile)
	errAccountGenerateChallengeErr = errors.New(errAccountGenerateChallenge)
	errAccountRegisterErr          = errors.New(errAccountRegister)
	errInvalidChallengeErr         = errors.New("Invalid challenge")
	errInvalidSignatureErr         = errors.New("Invalid signature")
	errPubkeyNotSupported          = errors.New("Only ed25519 keys are supported")
	errInvalidEmail                = errors.New("Invalid email")
	errEmailAlreadyExists          = errors.New("Email already exists")
	errGeneratingPassword          = errors.New("Error generating password")
	errPubkeyAlreadyExists         = errors.New("Pubkey already exists")
	errPubkeyNotExist              = errors.New("Pubkey does not exist")
	errAccountLoginErr             = errors.New(errAccountLogin)
	errFailedToGetPinsErr          = errors.New(errFailedToGetPins)
	errFailedToDelPinErr           = errors.New(errFailedToDelPin)
	errFailedToAddPinErr           = errors.New(errFailedToAddPin)
	errNotMultiformErr             = errors.New(errorNotMultiform)
	errFetchingUrlsErr             = errors.New(errFetchingUrls)
	errDownloadingFileErr          = errors.New(errDownloadingFile)
)

type HttpHandler struct {
	verifier *emailverifier.Verifier
	config   *viper.Viper
	logger   *zap.Logger
	storage  *storage.StorageServiceDefault
	db       *gorm.DB
	accounts *account.AccountServiceDefault
	protocol *s5.S5Protocol
}

type HttpHandlerParams struct {
	fx.In

	Config   *viper.Viper
	Logger   *zap.Logger
	Storage  *storage.StorageServiceDefault
	Db       *gorm.DB
	Accounts *account.AccountServiceDefault
	Protocol *s5.S5Protocol
}

type HttpHandlerResult struct {
	fx.Out

	HttpHandler HttpHandler
}

func NewHttpHandler(params HttpHandlerParams) (HttpHandlerResult, error) {
	return HttpHandlerResult{
		HttpHandler: HttpHandler{
			config:   params.Config,
			logger:   params.Logger,
			storage:  params.Storage,
			db:       params.Db,
			accounts: params.Accounts,
			protocol: params.Protocol,
		},
	}, nil
}

func (h *HttpHandler) smallFileUpload(jc jape.Context) {
	var rs io.ReadSeeker
	var bufferSize int64

	r := jc.Request
	contentType := r.Header.Get("Content-Type")

	if strings.HasPrefix(contentType, "multipart/form-data") {
		// Parse the multipart form
		err := r.ParseMultipartForm(h.config.GetInt64("core.post-upload-limit"))

		if jc.Check(errMultiformParse, err) != nil {
			h.logger.Error(errMultiformParse, zap.Error(err))
			return
		}

		// Retrieve the file from the form data
		file, _, err := r.FormFile("file")
		if jc.Check(errRetrievingFile, err) != nil {
			h.logger.Error(errRetrievingFile, zap.Error(err))
			return
		}
		defer func(file multipart.File) {
			err := file.Close()
			if err != nil {
				h.logger.Error(errClosingStream, zap.Error(err))
			}
		}(file)

		rs = file
	} else {
		data, err := io.ReadAll(r.Body)
		if jc.Check(errReadFile, err) != nil {
			h.logger.Error(errReadFile, zap.Error(err))
			return
		}

		buffer := bytes.NewReader(data)
		bufferSize = int64(buffer.Len())
		rs = buffer

		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				h.logger.Error(errClosingStream, zap.Error(err))
			}
		}(r.Body)
	}

	hash, err := h.storage.GetHashSmall(rs)
	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return
	}

	if exists, upload := h.storage.FileExists(hash.Hash); exists {
		cid, err := encoding.CIDFromHash(hash, upload.Size, types.CIDTypeRaw, types.HashTypeBlake3)
		if err != nil {
			_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
			h.logger.Error(errUploadingFile, zap.Error(err))
			return
		}
		cidStr, err := cid.ToString()
		if err != nil {
			_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
			h.logger.Error(errUploadingFile, zap.Error(err))
			return
		}

		err = h.accounts.PinByID(upload.ID, uint(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64)))
		if err != nil {
			_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
			h.logger.Error(errUploadingFile, zap.Error(err))
			return
		}

		jc.Encode(&SmallUploadResponse{
			CID: cidStr,
		})
		return
	}

	hash, err = h.storage.PutFileSmall(rs, "s5")

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return
	}

	h.logger.Info("Hash", zap.String("hash", hex.EncodeToString(hash.Hash)))

	cid, err := encoding.CIDFromHash(hash, uint64(bufferSize), types.CIDTypeRaw, types.HashTypeBlake3)

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return
	}

	cidStr, err := cid.ToString()

	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return
	}

	h.logger.Info("CID", zap.String("cidStr", cidStr))

	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return

	}

	var mimeBytes [512]byte

	_, err = rs.Read(mimeBytes[:])
	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return
	}

	mimeType := http.DetectContentType(mimeBytes[:])

	upload, err := h.storage.CreateUpload(hash.Hash, mimeType, uint(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64)), jc.Request.RemoteAddr, uint64(bufferSize), "s5")
	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
	}

	err = h.accounts.PinByID(upload.ID, uint(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64)))
	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
	}

	jc.Encode(&SmallUploadResponse{
		CID: cidStr,
	})
}

func (h *HttpHandler) accountRegisterChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	challenge := make([]byte, 32)

	_, err := rand.Read(challenge)
	if err != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.logger.Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)

	if err != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.logger.Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	if len(decodedKey) != 33 && int(decodedKey[0]) != int(types.HashTypeEd25519) {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.logger.Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	result := h.db.Create(&models.S5Challenge{
		Pubkey:    pubkey,
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "register",
	})

	if result.Error != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.logger.Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	jc.Encode(&AccountRegisterChallengeResponse{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
	})
}

func (h *HttpHandler) accountRegister(jc jape.Context) {
	var request AccountRegisterRequest

	if jc.Decode(&request) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errAccountRegisterErr, http.StatusInternalServerError)
		h.logger.Error(errAccountRegister, zap.Error(err))
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(request.Pubkey)

	if err != nil {
		errored(err)
		return
	}

	if len(decodedKey) != 33 {
		errored(err)
		return
	}

	var challenge models.S5Challenge

	result := h.db.Model(&models.S5Challenge{}).Where(&models.S5Challenge{Pubkey: request.Pubkey, Type: "register"}).First(&challenge)

	if result.RowsAffected == 0 || result.Error != nil {
		errored(err)
		return
	}

	decodedResponse, err := base64.RawURLEncoding.DecodeString(request.Response)

	if err != nil {
		errored(errInvalidChallengeErr)
		return
	}

	if len(decodedResponse) != 65 {
		errored(errInvalidChallengeErr)
		return
	}

	decodedChallenge, err := base64.RawURLEncoding.DecodeString(challenge.Challenge)

	if err != nil {
		errored(errInvalidChallengeErr)
		return
	}

	if !bytes.Equal(decodedResponse[1:33], decodedChallenge) {
		errored(errInvalidChallengeErr)
		return
	}

	if int(decodedKey[0]) != int(types.HashTypeEd25519) {
		errored(errPubkeyNotSupported)
		return
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(request.Signature)

	if err != nil {
		errored(err)
		return
	}

	if !ed25519.Verify(decodedKey[1:], decodedResponse, decodedSignature) {
		errored(errInvalidSignatureErr)
		return
	}

	if request.Email == "" {
		request.Email = fmt.Sprintf("%s@%s", hex.EncodeToString(decodedKey[1:]), "example.com")
	}

	accountExists, _, _ := h.accounts.EmailExists(request.Email)

	if accountExists {
		errored(errEmailAlreadyExists)
		return
	}

	pubkeyExists, _, _ := h.accounts.PubkeyExists(hex.EncodeToString(decodedKey[1:]))

	if pubkeyExists {
		errored(errPubkeyAlreadyExists)
		return
	}

	passwd := make([]byte, 32)

	_, err = rand.Read(passwd)

	if accountExists {
		errored(errGeneratingPassword)
		return
	}

	newAccount, err := h.accounts.CreateAccount(request.Email, string(passwd))
	if err != nil {
		errored(errAccountRegisterErr)
		return
	}

	rawPubkey := hex.EncodeToString(decodedKey[1:])

	err = h.accounts.AddPubkeyToAccount(*newAccount, rawPubkey)
	if err != nil {
		errored(errAccountRegisterErr)
		return
	}

	jwt, err := h.accounts.LoginPubkey(rawPubkey)
	if err != nil {
		errored(errAccountRegisterErr)
		return
	}

	result = h.db.Delete(&challenge)

	if result.Error != nil {
		errored(errAccountRegisterErr)
		return
	}

	setAuthCookie(jwt, jc)
}

func (h *HttpHandler) accountLoginChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errAccountLoginErr, http.StatusInternalServerError)
		h.logger.Error(errAccountLogin, zap.Error(err))
	}

	challenge := make([]byte, 32)

	_, err := rand.Read(challenge)
	if err != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.logger.Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)

	if err != nil {
		errored(errAccountGenerateChallengeErr)
		return
	}

	if len(decodedKey) != 33 && int(decodedKey[0]) != int(types.HashTypeEd25519) {
		errored(errPubkeyNotSupported)
		return
	}

	pubkeyExists, _, _ := h.accounts.PubkeyExists(hex.EncodeToString(decodedKey[1:]))

	if pubkeyExists {
		errored(errPubkeyNotExist)
		return
	}

	result := h.db.Create(&models.S5Challenge{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "login",
	})

	if result.Error != nil {
		errored(result.Error)
		return
	}

	jc.Encode(&AccountLoginChallengeResponse{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
	})
}

func (h *HttpHandler) accountLogin(jc jape.Context) {
	var request AccountLoginRequest

	if jc.Decode(&request) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errAccountLoginErr, http.StatusInternalServerError)
		h.logger.Error(errAccountLogin, zap.Error(err))
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(request.Pubkey)
	if err != nil {
		errored(err)
		return
	}

	if len(decodedKey) != 32 {
		errored(err)
		return
	}

	var challenge models.S5Challenge

	result := h.db.Model(&models.S5Challenge{}).Where(&models.S5Challenge{Pubkey: request.Pubkey, Type: "login"}).First(&challenge)

	if result.RowsAffected == 0 || result.Error != nil {
		errored(err)
		return
	}

	decodedResponse, err := base64.RawURLEncoding.DecodeString(request.Response)

	if err != nil {
		errored(err)
		return
	}

	if len(decodedResponse) != 65 {
		errored(err)
		return
	}

	decodedChallenge, err := base64.RawURLEncoding.DecodeString(challenge.Challenge)

	if err != nil {
		errored(err)
		return
	}

	if !bytes.Equal(decodedResponse[1:33], decodedChallenge) {
		errored(errInvalidChallengeErr)
		return
	}

	if int(decodedKey[0]) != int(types.HashTypeEd25519) {
		errored(errPubkeyNotSupported)
		return
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(request.Signature)

	if err != nil {
		errored(err)
		return
	}

	if !ed25519.Verify(decodedKey[1:], decodedResponse, decodedSignature) {
		errored(errInvalidSignatureErr)
		return
	}

	jwt, err := h.accounts.LoginPubkey(request.Pubkey)

	if err != nil {
		errored(errAccountLoginErr)
		return
	}

	result = h.db.Delete(&challenge)

	if result.Error != nil {
		errored(errAccountLoginErr)
		return
	}

	setAuthCookie(jwt, jc)
}

func (h *HttpHandler) accountInfo(jc jape.Context) {
	_, user, _ := h.accounts.AccountExists(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint))

	info := &AccountInfoResponse{
		Email:          user.Email,
		QuotaExceeded:  false,
		EmailConfirmed: false,
		IsRestricted:   false,
		Tier: AccountTier{
			Id:              1,
			Name:            "default",
			UploadBandwidth: math.MaxUint32,
			StorageLimit:    math.MaxUint32,
			Scopes:          []interface{}{},
		},
	}

	jc.Encode(info)
}

func (h *HttpHandler) accountStats(jc jape.Context) {
	_, user, _ := h.accounts.AccountExists(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint))

	info := &AccountStatsResponse{
		AccountInfoResponse: AccountInfoResponse{
			Email:          user.Email,
			QuotaExceeded:  false,
			EmailConfirmed: false,
			IsRestricted:   false,
			Tier: AccountTier{
				Id:              1,
				Name:            "default",
				UploadBandwidth: math.MaxUint32,
				StorageLimit:    math.MaxUint32,
				Scopes:          []interface{}{},
			},
		},
		Stats: AccountStats{
			Total: AccountStatsTotal{
				UsedStorage: 0,
			},
		},
	}

	jc.Encode(info)
}

func (h *HttpHandler) accountPins(jc jape.Context) {
	var cursor uint64

	if jc.DecodeForm("cursor", &cursor) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errFailedToGetPinsErr, http.StatusInternalServerError)
		h.logger.Error(errFailedToGetPins, zap.Error(err))
	}

	pins, err := h.accounts.AccountPins(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64), cursor)

	if err != nil {
		errored(err)
		return
	}

	pinResponse := &AccountPinResponse{Cursor: cursor, Pins: pins}

	result, err := msgpack.Marshal(pinResponse)

	if err != nil {
		errored(err)
		return
	}

	jc.Custom(jc.Request, result)

	jc.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = jc.ResponseWriter.Write(result)
}

func (h *HttpHandler) accountPinDelete(jc jape.Context) {
	var cid string
	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errFailedToDelPinErr, http.StatusInternalServerError)
		h.logger.Error(errFailedToDelPin, zap.Error(err))
	}

	decodedCid, err := encoding.CIDFromString(cid)

	if err != nil {
		errored(err)
		return
	}

	hash := hex.EncodeToString(decodedCid.Hash.HashBytes())

	err = h.accounts.DeletePinByHash(hash, uint(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64)))

	if err != nil {
		errored(err)
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (h *HttpHandler) accountPin(jc jape.Context) {
	var cid string
	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errFailedToAddPinErr, http.StatusInternalServerError)
		h.logger.Error(errFailedToAddPin, zap.Error(err))
	}

	decodedCid, err := encoding.CIDFromString(cid)

	if err != nil {
		errored(err)
		return
	}

	h.logger.Info("CID", zap.String("cidStr", cid))
	h.logger.Info("hash", zap.String("hash", hex.EncodeToString(decodedCid.Hash.HashBytes())))

	hash := hex.EncodeToString(decodedCid.Hash.HashBytes())

	err = h.accounts.PinByHash(hash, uint(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64)))

	if err != nil {
		errored(err)
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (h *HttpHandler) directoryUpload(jc jape.Context) {
	var tryFiles []string
	var errorPages map[int]string
	var name string

	if jc.DecodeForm("tryFiles", &tryFiles) != nil {
		return
	}

	if jc.DecodeForm("errorPages", &errorPages) != nil {
		return
	}

	if jc.DecodeForm("name", &name) != nil {
		return
	}

	r := jc.Request
	contentType := r.Header.Get("Content-Type")

	errored := func(err error) {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
	}

	if !strings.HasPrefix(contentType, "multipart/form-data") {
		_ = jc.Error(errNotMultiformErr, http.StatusBadRequest)
		h.logger.Error(errorNotMultiform)
		return
	}

	err := r.ParseMultipartForm(h.config.GetInt64("core.post-upload-limit"))

	if jc.Check(errMultiformParse, err) != nil {
		h.logger.Error(errMultiformParse, zap.Error(err))
		return
	}

	uploadMap := make(map[string]models.Upload, len(r.MultipartForm.File))
	mimeMap := make(map[string]string, len(r.MultipartForm.File))

	for _, files := range r.MultipartForm.File {
		for _, fileHeader := range files {
			// Open the file.
			file, err := fileHeader.Open()
			if err != nil {
				errored(err)
				return
			}
			defer func(file multipart.File) {
				err := file.Close()
				if err != nil {
					h.logger.Error(errClosingStream, zap.Error(err))
				}
			}(file)

			var rs io.ReadSeeker

			hash, err := h.storage.GetHashSmall(rs)
			_, err = rs.Seek(0, io.SeekStart)
			if err != nil {
				_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
				h.logger.Error(errUploadingFile, zap.Error(err))
				return
			}

			if exists, upload := h.storage.FileExists(hash.Hash); exists {
				uploadMap[fileHeader.Filename] = upload
				continue
			}

			hash, err = h.storage.PutFileSmall(rs, "s5")

			if err != nil {
				errored(err)
				return
			}

			_, err = rs.Seek(0, io.SeekStart)
			if err != nil {
				return
			}

			if err != nil {
				errored(err)
				return
			}

			var mimeBytes [512]byte

			if _, err := file.Read(mimeBytes[:]); err != nil {
				errored(err)
				return
			}
			mimeType := http.DetectContentType(mimeBytes[:])

			upload, err := h.storage.CreateUpload(hash.Hash, mimeType, uint(jc.Request.Context().Value(middleware.S5AuthUserIDKey).(uint64)), jc.Request.RemoteAddr, uint64(fileHeader.Size), "s5")

			if err != nil {
				errored(err)
				return
			}

			// Reset the read pointer back to the start of the file.
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				errored(err)
				return
			}

			// Read a snippet of the file to determine its MIME type.
			buffer := make([]byte, 512) // 512 bytes should be enough for http.DetectContentType to determine the type
			if _, err := file.Read(buffer); err != nil {
				errored(err)
				return
			}

			uploadMap[fileHeader.Filename] = *upload
			mimeMap[fileHeader.Filename] = mimeType
		}
	}
	filesMap := make(map[string]metadata.WebAppMetadataFileReference, len(uploadMap))

	for name, file := range uploadMap {
		hashDecoded, err := hex.DecodeString(file.Hash)
		if err != nil {
			errored(err)
			return
		}

		cid, err := encoding.CIDFromHash(hashDecoded, file.Size, types.CIDTypeRaw, types.HashTypeBlake3)
		if err != nil {
			errored(err)
			return
		}

		filesMap[name] = *metadata.NewWebAppMetadataFileReference(cid, mimeMap[name])
	}

	app := metadata.NewWebAppMetadata(name, tryFiles, *metadata.NewExtraMetadata(map[int]interface{}{}), errorPages, filesMap)

	appData, err := msgpack.Marshal(app)
	if err != nil {
		errored(err)
		return
	}

	var rs = bytes.NewReader(appData)

	hash, err := h.storage.GetHashSmall(rs)
	_, err = rs.Seek(0, io.SeekStart)
	if err != nil {
		_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
		h.logger.Error(errUploadingFile, zap.Error(err))
		return
	}

	if exists, upload := h.storage.FileExists(hash.Hash); exists {
		cid, err := encoding.CIDFromHash(hash, upload.Size, types.CIDTypeMetadataWebapp, types.HashTypeBlake3)
		if err != nil {
			_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
			h.logger.Error(errUploadingFile, zap.Error(err))
			return
		}
		cidStr, err := cid.ToString()
		if err != nil {
			_ = jc.Error(errUploadingFileErr, http.StatusInternalServerError)
			h.logger.Error(errUploadingFile, zap.Error(err))
			return
		}
		jc.Encode(map[string]string{"hash": cidStr})
		return
	}

	hash, err = h.storage.PutFileSmall(rs, "s5")

	if err != nil {
		errored(err)
		return
	}

	cid, err := encoding.CIDFromHash(hash, uint64(len(appData)), types.CIDTypeRaw, types.HashTypeBlake3)

	if err != nil {
		errored(err)
		return
	}

	cidStr, err := cid.ToString()
	if err != nil {
		errored(err)
		return
	}

	jc.Encode(&AppUploadResponse{CID: cidStr})
}

func (h *HttpHandler) debugDownloadUrls(jc jape.Context) {
	var cid string
	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	decodedCid, err := encoding.CIDFromString(cid)

	if err != nil {
		_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
		h.logger.Error(errFetchingUrls, zap.Error(err))
		return
	}

	node := h.getNode()

	dlUriProvider := h.newStorageLocationProvider(&decodedCid.Hash, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

	err = dlUriProvider.Start()
	if err != nil {
		_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
		h.logger.Error(errFetchingUrls, zap.Error(err))
		return
	}

	_, err = dlUriProvider.Next()
	if err != nil {
		_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
		h.logger.Error(errFetchingUrls, zap.Error(err))
		return
	}

	locations, err := node.Services().Storage().GetCachedStorageLocations(&decodedCid.Hash, []types.StorageLocationType{
		types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge,
	})
	if err != nil {
		_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
		h.logger.Error(errFetchingUrls, zap.Error(err))
		return
	}

	availableNodes := lo.Keys[string, libs5storage.StorageLocation](locations)

	availableNodesIds := make([]*encoding.NodeId, len(availableNodes))

	for i, nodeIdStr := range availableNodes {
		nodeId, err := encoding.DecodeNodeId(nodeIdStr)
		if err != nil {
			_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
			h.logger.Error(errFetchingUrls, zap.Error(err))
			return
		}
		availableNodesIds[i] = nodeId
	}

	sorted, err := node.Services().P2P().SortNodesByScore(availableNodesIds)
	if err != nil {
		return
	}

	if err != nil {
		_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
		h.logger.Error(errFetchingUrls, zap.Error(err))
		return
	}

	output := make([]string, len(sorted))

	for i, nodeId := range sorted {
		nodeIdStr, err := nodeId.ToString()
		if err != nil {
			_ = jc.Error(errFetchingUrlsErr, http.StatusInternalServerError)
			h.logger.Error(errFetchingUrls, zap.Error(err))
			return
		}
		output[i] = locations[nodeIdStr].BytesURL()
	}

	jc.ResponseWriter.WriteHeader(http.StatusOK)
	_, _ = jc.ResponseWriter.Write([]byte(strings.Join(output, "\n")))
}

func (h *HttpHandler) registryQuery(jc jape.Context) {
	var pk string

	if jc.DecodeForm("pk", &pk) != nil {
		return
	}

	pkBytes, err := base64.RawURLEncoding.DecodeString(pk)
	if jc.Check("error decoding pk", err) != nil {
		return
	}

	entry, err := h.getNode().Services().Registry().Get(pkBytes)
	if jc.Check("error getting registry entry", err) != nil {
		return
	}

	if entry == nil {
		jc.ResponseWriter.WriteHeader(http.StatusNotFound)
		return
	}

	jc.Encode(&RegistryQueryResponse{
		Pk:        base64.RawURLEncoding.EncodeToString(entry.PK()),
		Revision:  entry.Revision(),
		Data:      base64.RawURLEncoding.EncodeToString(entry.Data()),
		Signature: base64.RawURLEncoding.EncodeToString(entry.Signature()),
	})
}

func (h *HttpHandler) registrySet(jc jape.Context) {
	var request RegistrySetRequest

	if jc.Decode(&request) != nil {
		return
	}

	pk, err := base64.RawURLEncoding.DecodeString(request.Pk)
	if jc.Check("error decoding pk", err) != nil {
		return
	}

	data, err := base64.RawURLEncoding.DecodeString(request.Data)
	if jc.Check("error decoding data", err) != nil {
		return
	}

	signature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if jc.Check("error decoding signature", err) != nil {
		return
	}

	entry := libs5protocol.NewSignedRegistryEntry(pk, request.Revision, data, signature)

	err = h.getNode().Services().Registry().Set(entry, false, nil)
	if jc.Check("error setting registry entry", err) != nil {
		return
	}
}

func (h *HttpHandler) registrySubscription(jc jape.Context) {
	// Create a context for the WebSocket operations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var listeners []func()

	// Accept the WebSocket connection
	c, err := websocket.Accept(jc.ResponseWriter, jc.Request, nil)
	if err != nil {
		h.logger.Error("error accepting websocket connection", zap.Error(err))
		return
	}
	defer func(c *websocket.Conn, code websocket.StatusCode, reason string) {
		err := c.Close(code, reason)
		if err != nil {
			h.logger.Error("error closing websocket connection", zap.Error(err))
		}

		for _, listener := range listeners {
			listener()
		}
	}(c, websocket.StatusNormalClosure, "connection closed")

	// Main loop for reading messages
	for {
		// Read a message (the actual reading and unpacking is skipped here)
		_, data, err := c.Read(ctx)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				// Normal closure
				h.logger.Info("websocket connection closed normally")
			} else {
				// Handle different types of errors
				h.logger.Error("error in websocket connection", zap.Error(err))
			}
			break
		}

		decoder := msgpack.NewDecoder(bytes.NewReader(data))

		method, err := decoder.DecodeInt()

		if err != nil {
			h.logger.Error("error decoding method", zap.Error(err))
			break
		}

		if method != 2 {
			h.logger.Error("invalid method", zap.Int64("method", int64(method)))
			break
		}

		sre, err := decoder.DecodeBytes()

		if err != nil {
			h.logger.Error("error decoding sre", zap.Error(err))
			break
		}

		off, err := h.getNode().Services().Registry().Listen(sre, func(entry libs5protocol.SignedRegistryEntry) {
			encoded, err := msgpack.Marshal(entry)
			if err != nil {
				h.logger.Error("error encoding entry", zap.Error(err))
				return
			}

			err = c.Write(ctx, websocket.MessageBinary, encoded)

			if err != nil {
				h.logger.Error("error writing to websocket", zap.Error(err))
			}
		})
		if err != nil {
			h.logger.Error("error listening to registry", zap.Error(err))
			break
		}

		listeners = append(listeners, off)
	}
}

func (h *HttpHandler) getNode() *libs5node.Node {
	return h.protocol.Node()
}

func (h *HttpHandler) downloadBlob(jc jape.Context) {
	var cid string

	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	cid = strings.Split(cid, ".")[0]

	cidDecoded, err := encoding.CIDFromString(cid)
	if jc.Check("error decoding cid", err) != nil {
		return
	}

	dlUriProvider := h.newStorageLocationProvider(&cidDecoded.Hash, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

	err = dlUriProvider.Start()

	if jc.Check("error starting search", err) != nil {
		return
	}

	next, err := dlUriProvider.Next()
	if jc.Check("error fetching blob", err) != nil {
		return
	}

	http.Redirect(jc.ResponseWriter, jc.Request, next.Location().BytesURL(), http.StatusFound)
}

func (h *HttpHandler) debugStorageLocations(jc jape.Context) {
	var hash string

	if jc.DecodeParam("hash", &hash) != nil {
		return
	}

	var kinds string

	if jc.DecodeForm("kinds", &kinds) != nil {
		return
	}

	decodedHash, err := encoding.MultihashFromBase64Url(hash)
	if jc.Check("error decoding hash", err) != nil {
		return
	}

	typeList := strings.Split(kinds, ",")
	typeIntList := make([]types.StorageLocationType, 0)

	for _, typeStr := range typeList {
		typeInt, err := strconv.Atoi(typeStr)
		if err != nil {
			continue
		}
		typeIntList = append(typeIntList, types.StorageLocationType(typeInt))
	}

	if len(typeIntList) == 0 {
		typeIntList = []types.StorageLocationType{
			types.StorageLocationTypeFull,
			types.StorageLocationTypeFile,
			types.StorageLocationTypeBridge,
			types.StorageLocationTypeArchive,
		}
	}

	dlUriProvider := h.newStorageLocationProvider(decodedHash, typeIntList...)

	err = dlUriProvider.Start()
	if jc.Check("error starting search", err) != nil {
		return
	}

	_, err = dlUriProvider.Next()
	if jc.Check("error fetching locations", err) != nil {
		return
	}

	locations, err := h.getNode().Services().Storage().GetCachedStorageLocations(decodedHash, typeIntList)
	if jc.Check("error getting cached locations", err) != nil {
		return
	}

	availableNodes := lo.Keys[string, libs5storage.StorageLocation](locations)
	availableNodesIds := make([]*encoding.NodeId, len(availableNodes))

	for i, nodeIdStr := range availableNodes {
		nodeId, err := encoding.DecodeNodeId(nodeIdStr)
		if jc.Check("error decoding node id", err) != nil {
			return
		}
		availableNodesIds[i] = nodeId
	}

	availableNodesIds, err = h.getNode().Services().P2P().SortNodesByScore(availableNodesIds)

	if jc.Check("error sorting nodes", err) != nil {
		return
	}

	debugLocations := make([]DebugStorageLocation, len(availableNodes))

	for i, nodeId := range availableNodesIds {
		nodeIdStr, err := nodeId.ToBase58()
		if jc.Check("error encoding node id", err) != nil {
			return
		}

		score, err := h.getNode().Services().P2P().GetNodeScore(nodeId)

		if jc.Check("error getting node score", err) != nil {
			return
		}

		debugLocations[i] = DebugStorageLocation{
			Type:   locations[nodeIdStr].Type(),
			Parts:  locations[nodeIdStr].Parts(),
			Expiry: locations[nodeIdStr].Expiry(),
			NodeId: nodeIdStr,
			Score:  score,
		}
	}

	jc.Encode(&DebugStorageLocationsResponse{
		Locations: debugLocations,
	})
}

func (h *HttpHandler) downloadMetadata(jc jape.Context) {
	var cid string

	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	cidDecoded, err := encoding.CIDFromString(cid)
	if jc.Check("error decoding cid", err) != nil {
		h.logger.Error("error decoding cid", zap.Error(err))
		return
	}

	switch cidDecoded.Type {
	case types.CIDTypeRaw:
		_ = jc.Error(errors.New("Raw CIDs do not have metadata"), http.StatusBadRequest)
		return

	case types.CIDTypeResolver:
		_ = jc.Error(errors.New("Resolver CIDs not yet supported"), http.StatusBadRequest)
		return
	}

	meta, err := h.getNode().Services().Storage().GetMetadataByCID(cidDecoded)

	if jc.Check("error getting metadata", err) != nil {
		h.logger.Error("error getting metadata", zap.Error(err))
		return
	}

	if cidDecoded.Type != types.CIDTypeBridge {
		jc.ResponseWriter.Header().Set("Cache-Control", "public, max-age=31536000")
	} else {
		jc.ResponseWriter.Header().Set("Cache-Control", "public, max-age=60")
	}

	jc.Encode(&meta)

}

func (h *HttpHandler) downloadFile(jc jape.Context) {
	var cid string

	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	var hashBytes []byte
	isProof := false

	if strings.HasSuffix(cid, ".obao") {
		isProof = true
		cid = strings.TrimSuffix(cid, ".obao")
	}

	cidDecoded, err := encoding.CIDFromString(cid)

	if err != nil {
		hashDecoded, err := encoding.MultihashFromBase64Url(cid)

		if jc.Check("error decoding as cid or hash", err) != nil {
			return
		}

		hashBytes = hashDecoded.HashBytes()
	} else {
		hashBytes = cidDecoded.Hash.HashBytes()
	}

	file := h.storage.NewFile(hashBytes)

	if !file.Exists() {
		jc.ResponseWriter.WriteHeader(http.StatusNotFound)
		return
	}

	defer func(file io.ReadCloser) {
		err := file.Close()
		if err != nil {
			h.logger.Error("error closing file", zap.Error(err))
		}
	}(file)

	if isProof {
		proof, err := file.Proof()

		if jc.Check("error getting proof", err) != nil {
			return
		}

		jc.ResponseWriter.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(jc.ResponseWriter, jc.Request, fmt.Sprintf("%.obao", file.Name()), file.Modtime(), bytes.NewReader(proof))
		return
	}

	jc.ResponseWriter.Header().Set("Content-Type", file.Mime())

	http.ServeContent(jc.ResponseWriter, jc.Request, file.Name(), file.Modtime(), file)
}

func (h *HttpHandler) newStorageLocationProvider(hash *encoding.Multihash, types ...types.StorageLocationType) libs5storage.StorageLocationProvider {
	return libs5storageProvider.NewStorageLocationProvider(libs5storageProvider.StorageLocationProviderParams{
		Services:      h.getNode().Services(),
		Hash:          hash,
		LocationTypes: types,
		ServiceParams: libs5service.ServiceParams{
			Logger: h.logger,
			Config: h.getNode().Config(),
			Db:     h.getNode().Db(),
		},
	})
}

func setAuthCookie(jwt string, jc jape.Context) {
	authCookie := http.Cookie{
		Name:     "s5-auth-token",
		Value:    jwt,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(time.Hour.Seconds() * 24),
		Secure:   true,
	}

	http.SetCookie(jc.ResponseWriter, &authCookie)
}

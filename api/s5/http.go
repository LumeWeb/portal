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
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"git.lumeweb.com/LumeWeb/libs5-go/metadata"

	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
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
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"github.com/vmihailenco/msgpack/v5"
	"go.sia.tech/jape"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nhooyr.io/websocket"
)

type readSeekNopCloser struct {
	*bytes.Reader
}

func (rsnc readSeekNopCloser) Close() error {
	return nil
}

type HttpHandler struct {
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
	user := middleware.GetUserFromContext(jc.Request.Context())

	file, err := h.prepareFileUpload(jc)
	if err != nil {
		h.sendErrorResponse(jc, err)
		return
	}
	defer func(file io.ReadSeekCloser) {
		err := file.Close()
		if err != nil {
			h.logger.Error("Error closing file", zap.Error(err))
		}
	}(file)

	// Use PutFileSmall for the actual file upload
	newUpload, err2 := h.storage.PutFileSmall(file, "s5", user, jc.Request.RemoteAddr)
	if err2 != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	cid, err2 := encoding.CIDFromHash(newUpload.Hash, newUpload.Size, types.CIDTypeRaw, types.HashTypeBlake3)
	if err2 != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	cidStr, err2 := cid.ToString()
	if err2 != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	jc.Encode(&SmallUploadResponse{
		CID: cidStr,
	})
}

func (h *HttpHandler) prepareFileUpload(jc jape.Context) (file io.ReadSeekCloser, s5Err *S5Error) {
	r := jc.Request
	contentType := r.Header.Get("Content-Type")

	// Handle multipart form data uploads
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(h.config.GetInt64("core.post-upload-limit")); err != nil {
			return nil, NewS5Error(ErrKeyFileUploadFailed, err)
		}

		multipartFile, _, err := r.FormFile("file")
		if err != nil {
			return nil, NewS5Error(ErrKeyFileUploadFailed, err)
		}

		return multipartFile, nil
	}

	// Handle raw body uploads
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, NewS5Error(ErrKeyFileUploadFailed, err)
	}

	buffer := readSeekNopCloser{bytes.NewReader(data)}

	return buffer, nil
}

func (h *HttpHandler) accountRegisterChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	challenge := make([]byte, 32)
	_, err := rand.Read(challenge)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	if len(decodedKey) != 33 || int(decodedKey[0]) != int(types.HashTypeEd25519) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyDataIntegrityError, fmt.Errorf("invalid public key format")))
		return
	}

	result := h.db.Create(&models.S5Challenge{
		Pubkey:    pubkey,
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "register",
	})

	if result.Error != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
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

	decodedKey, err := base64.RawURLEncoding.DecodeString(request.Pubkey)
	if err != nil || len(decodedKey) != 33 || int(decodedKey[0]) != int(types.HashTypeEd25519) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	challenge := models.S5Challenge{
		Pubkey: request.Pubkey,
		Type:   "register",
	}

	if result := h.db.Where(&challenge).First(&challenge); result.RowsAffected == 0 || result.Error != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, result.Error))
		return
	}

	decodedResponse, err := base64.RawURLEncoding.DecodeString(request.Response)
	if err != nil || len(decodedResponse) != 65 {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyDataIntegrityError, err))
		return
	}

	decodedChallenge, err := base64.RawURLEncoding.DecodeString(challenge.Challenge)
	if err != nil || !bytes.Equal(decodedResponse[1:33], decodedChallenge) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil || !ed25519.Verify(decodedKey[1:], decodedResponse, decodedSignature) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyAuthorizationFailed, err))
		return
	}

	if request.Email == "" {
		request.Email = fmt.Sprintf("%s@%s", hex.EncodeToString(decodedKey[1:]), "example.com")
	}

	if accountExists, _, _ := h.accounts.EmailExists(request.Email); accountExists {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyResourceLimitExceeded, fmt.Errorf("email already exists")))
		return
	}

	if pubkeyExists, _, _ := h.accounts.PubkeyExists(hex.EncodeToString(decodedKey[1:])); pubkeyExists {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyResourceLimitExceeded, fmt.Errorf("pubkey already exists")))
		return
	}

	passwd := make([]byte, 32)
	if _, err = rand.Read(passwd); err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
		return
	}

	newAccount, err := h.accounts.CreateAccount(request.Email, string(passwd))
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	rawPubkey := hex.EncodeToString(decodedKey[1:])
	if err = h.accounts.AddPubkeyToAccount(*newAccount, rawPubkey); err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jwt, err := h.accounts.LoginPubkey(rawPubkey)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyAuthenticationFailed, err))
		return
	}

	if result := h.db.Delete(&challenge); result.Error != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	setAuthCookie(jwt, jc)
}

func (h *HttpHandler) accountLoginChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	challenge := make([]byte, 32)
	_, err := rand.Read(challenge)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	if len(decodedKey) != 33 || int(decodedKey[0]) != int(types.HashTypeEd25519) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyUnsupportedFileType, fmt.Errorf("public key not supported")))
		return
	}

	pubkeyExists, _, _ := h.accounts.PubkeyExists(hex.EncodeToString(decodedKey[1:]))
	if !pubkeyExists {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, fmt.Errorf("public key does not exist")))
		return
	}

	result := h.db.Create(&models.S5Challenge{
		Pubkey:    pubkey,
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "login",
	})

	if result.Error != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
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

	decodedKey, err := base64.RawURLEncoding.DecodeString(request.Pubkey)
	if err != nil || len(decodedKey) != 32 {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	if int(decodedKey[0]) != int(types.HashTypeEd25519) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyUnsupportedFileType, fmt.Errorf("public key type not supported")))
		return
	}

	var challenge models.S5Challenge
	result := h.db.Where(&models.S5Challenge{Pubkey: request.Pubkey, Type: "login"}).First(&challenge)
	if result.RowsAffected == 0 || result.Error != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, result.Error))
		return
	}

	decodedResponse, err := base64.RawURLEncoding.DecodeString(request.Response)
	if err != nil || len(decodedResponse) != 65 {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	decodedChallenge, err := base64.RawURLEncoding.DecodeString(challenge.Challenge)
	if err != nil || !bytes.Equal(decodedResponse[1:33], decodedChallenge) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyDataIntegrityError, err))
		return
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil || !ed25519.Verify(decodedKey[1:], decodedResponse, decodedSignature) {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyAuthorizationFailed, err))
		return
	}

	jwt, err := h.accounts.LoginPubkey(hex.EncodeToString(decodedKey[1:])) // Adjust based on how LoginPubkey is implemented
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyAuthenticationFailed, err))
		return
	}

	if result := h.db.Delete(&challenge); result.Error != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	setAuthCookie(jwt, jc)
}

func (h *HttpHandler) accountInfo(jc jape.Context) {
	userID := middleware.GetUserFromContext(jc.Request.Context())
	_, user, _ := h.accounts.AccountExists(userID)

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
	userID := middleware.GetUserFromContext(jc.Request.Context())
	_, user, _ := h.accounts.AccountExists(userID)

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
	if err := jc.DecodeForm("cursor", &cursor); err != nil {
		// Assuming jc.DecodeForm sends out its own error, so no need for further action here
		return
	}

	userID := middleware.GetUserFromContext(jc.Request.Context())

	pins, err := h.accounts.AccountPins(userID, cursor)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	pinResponse := &AccountPinResponse{Cursor: cursor, Pins: pins}
	result, err2 := msgpack.Marshal(pinResponse)
	if err2 != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err2))
		return
	}

	jc.ResponseWriter.Header().Set("Content-Type", "application/msgpack")
	jc.ResponseWriter.WriteHeader(http.StatusOK)
	if _, err := jc.ResponseWriter.Write(result); err != nil {
		h.logger.Error("failed to write account pins response", zap.Error(err))
	}
}

func (h *HttpHandler) accountPinDelete(jc jape.Context) {
	var cid string
	if err := jc.DecodeParam("cid", &cid); err != nil {
		return
	}

	user := middleware.GetUserFromContext(jc.Request.Context())

	decodedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	hash := hex.EncodeToString(decodedCid.Hash.HashBytes())
	if err := h.accounts.DeletePinByHash(hash, user); err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (h *HttpHandler) accountPin(jc jape.Context) {
	var cid string
	if err := jc.DecodeParam("cid", &cid); err != nil {
		return
	}

	userID := middleware.GetUserFromContext(jc.Request.Context())

	decodedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	hash := hex.EncodeToString(decodedCid.Hash.HashBytes())
	h.logger.Info("Processing pin request", zap.String("cid", cid), zap.String("hash", hash))

	if err := h.accounts.PinByHash(hash, userID); err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (h *HttpHandler) directoryUpload(jc jape.Context) {
	// Decode form fields
	var (
		tryFiles   []string
		errorPages map[int]string
		name       string
	)

	if err := jc.DecodeForm("tryFiles", &tryFiles); err != nil || jc.DecodeForm("errorPages", &errorPages) != nil || jc.DecodeForm("name", &name) != nil {
	}

	// Verify content type
	if contentType := jc.Request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "multipart/form-data") {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, fmt.Errorf("expected multipart/form-data content type, got %s", contentType)))
		return
	}

	// Parse multipart form with size limit from config
	if err := jc.Request.ParseMultipartForm(h.config.GetInt64("core.post-upload-limit")); err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	user := middleware.GetUserFromContext(jc.Request.Context())
	uploads, err := h.processMultipartFiles(jc.Request, user)
	if err != nil {
		h.sendErrorResponse(jc, err) // processMultipartFiles should return a properly wrapped S5Error
		return
	}

	// Generate metadata for the directory upload
	app, err := h.createAppMetadata(name, tryFiles, errorPages, uploads)
	if err != nil {
		h.sendErrorResponse(jc, err) // createAppMetadata should return a properly wrapped S5Error
		return
	}

	// Upload the metadata
	cidStr, err := h.uploadAppMetadata(app, user, jc.Request)
	if err != nil {
		h.sendErrorResponse(jc, err) // uploadAppMetadata should return a properly wrapped S5Error
		return
	}

	jc.Encode(&AppUploadResponse{CID: cidStr})
}

func (h *HttpHandler) processMultipartFiles(r *http.Request, user uint) (map[string]*models.Upload, error) {
	uploadMap := make(map[string]*models.Upload)

	for _, files := range r.MultipartForm.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, NewS5Error(ErrKeyStorageOperationFailed, err)
			}
			defer func(file multipart.File) {
				err := file.Close()
				if err != nil {
					h.logger.Error("Error closing file", zap.Error(err))
				}
			}(file)

			upload, err := h.storage.PutFileSmall(file, "s5", user, r.RemoteAddr)
			if err != nil {
				return nil, NewS5Error(ErrKeyStorageOperationFailed, err)
			}

			uploadMap[fileHeader.Filename] = upload
		}
	}

	return uploadMap, nil
}

func (h *HttpHandler) createAppMetadata(name string, tryFiles []string, errorPages map[int]string, uploads map[string]*models.Upload) (*metadata.WebAppMetadata, error) {
	filesMap := make(map[string]metadata.WebAppMetadataFileReference, len(uploads))

	for filename, upload := range uploads {
		hashDecoded, err := hex.DecodeString(upload.Hash)
		if err != nil {
			return nil, NewS5Error(ErrKeyInternalError, err, "Failed to decode hash for file: "+filename)
		}

		cid, err := encoding.CIDFromHash(hashDecoded, upload.Size, types.CIDTypeRaw, types.HashTypeBlake3)
		if err != nil {
			return nil, NewS5Error(ErrKeyInternalError, err, "Failed to create CID for file: "+filename)
		}
		filesMap[filename] = metadata.WebAppMetadataFileReference{
			Cid:         cid,
			ContentType: upload.MimeType,
		}
	}

	extraMetadataMap := make(map[int]interface{})
	for statusCode, page := range errorPages {
		extraMetadataMap[statusCode] = page
	}

	extraMetadata := metadata.NewExtraMetadata(extraMetadataMap)
	// Create the web app metadata object
	app := metadata.NewWebAppMetadata(
		name,
		tryFiles,
		*extraMetadata,
		errorPages,
		filesMap,
	)

	return app, nil
}

func (h *HttpHandler) uploadAppMetadata(appData *metadata.WebAppMetadata, userId uint, r *http.Request) (string, error) {
	appDataRaw, err := msgpack.Marshal(appData)
	if err != nil {
		return "", NewS5Error(ErrKeyInternalError, err, "Failed to marshal app metadata")
	}

	file := bytes.NewReader(appDataRaw)

	upload, err := h.storage.PutFileSmall(file, "s5", userId, r.RemoteAddr)
	if err != nil {
		return "", NewS5Error(ErrKeyStorageOperationFailed, err)
	}

	// Construct the CID for the newly uploaded metadata
	cid, err := encoding.CIDFromHash(upload.Hash, uint64(len(appDataRaw)), types.CIDTypeMetadataWebapp, types.HashTypeBlake3)
	if err != nil {
		return "", NewS5Error(ErrKeyInternalError, err, "Failed to create CID for new app metadata")
	}
	cidStr, err := cid.ToString()
	if err != nil {
		return "", NewS5Error(ErrKeyInternalError, err, "Failed to convert CID to string for new app metadata")
	}

	return cidStr, nil
}

func (h *HttpHandler) debugDownloadUrls(jc jape.Context) {
	var cid string
	if err := jc.DecodeParam("cid", &cid); err != nil {
		return
	}

	decodedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err, "Failed to decode CID"))
		return
	}

	node := h.getNode()
	dlUriProvider := h.newStorageLocationProvider(&decodedCid.Hash, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

	if err := dlUriProvider.Start(); err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Failed to start URI provider"))
		return
	}

	locations, err := node.Services().Storage().GetCachedStorageLocations(&decodedCid.Hash, []types.StorageLocationType{
		types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge,
	})
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Failed to get cached storage locations"))
		return
	}

	availableNodes := lo.Keys[string, libs5storage.StorageLocation](locations)
	availableNodesIds := make([]*encoding.NodeId, len(availableNodes))

	for i, nodeIdStr := range availableNodes {
		nodeId, err := encoding.DecodeNodeId(nodeIdStr)
		if err != nil {
			h.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err, "Failed to decode node ID"))
			return
		}
		availableNodesIds[i] = nodeId
	}

	sorted, err := node.Services().P2P().SortNodesByScore(availableNodesIds)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyNetworkError, err, "Failed to sort nodes by score"))
		return
	}

	output := make([]string, len(sorted))
	for i, nodeId := range sorted {
		nodeIdStr, err := nodeId.ToString()
		if err != nil {
			h.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err, "Failed to convert node ID to string"))
			return
		}
		output[i] = locations[nodeIdStr].BytesURL()
	}

	jc.ResponseWriter.WriteHeader(http.StatusOK)
	_, err = jc.ResponseWriter.Write([]byte(strings.Join(output, "\n")))
	if err != nil {
		h.logger.Error("Failed to write response", zap.Error(err))
	}
}

func (h *HttpHandler) registryQuery(jc jape.Context) {
	var pk string
	if err := jc.DecodeForm("pk", &pk); err != nil {
		return
	}

	pkBytes, err := base64.RawURLEncoding.DecodeString(pk)
	if err != nil {
		s5Err := NewS5Error(ErrKeyInvalidFileFormat, err)
		h.sendErrorResponse(jc, s5Err)
		return
	}

	entry, err := h.getNode().Services().Registry().Get(pkBytes)
	if err != nil {
		s5ErrKey := ErrKeyStorageOperationFailed
		s5Err := NewS5Error(s5ErrKey, err)
		h.sendErrorResponse(jc, s5Err)
		return
	}

	if entry == nil {
		jc.ResponseWriter.WriteHeader(http.StatusNotFound)
		return
	}

	response := RegistryQueryResponse{
		Pk:        base64.RawURLEncoding.EncodeToString(entry.PK()),
		Revision:  entry.Revision(),
		Data:      base64.RawURLEncoding.EncodeToString(entry.Data()),
		Signature: base64.RawURLEncoding.EncodeToString(entry.Signature()),
	}
	jc.Encode(response)
}

func (h *HttpHandler) registrySet(jc jape.Context) {
	var request RegistrySetRequest

	if err := jc.Decode(&request); err != nil {
		return
	}

	pk, err := base64.RawURLEncoding.DecodeString(request.Pk)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err, "Error decoding public key"))
		return
	}

	data, err := base64.RawURLEncoding.DecodeString(request.Data)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err, "Error decoding data"))
		return
	}

	signature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err, "Error decoding signature"))
		return
	}

	entry := libs5protocol.NewSignedRegistryEntry(pk, request.Revision, data, signature)

	err = h.getNode().Services().Registry().Set(entry, false, nil)
	if err != nil {
		h.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Error setting registry entry"))
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
	defer func() {
		// Close the WebSocket connection gracefully
		err := c.Close(websocket.StatusNormalClosure, "connection closed")
		if err != nil {
			h.logger.Error("error closing websocket connection", zap.Error(err))
		}
		// Clean up all listeners when the connection is closed
		for _, listener := range listeners {
			listener()
		}
	}()

	// Main loop for reading messages
	for {
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

		// Assuming method indicates the type of operation, validate it
		method, err := decoder.DecodeInt()
		if err != nil {
			h.logger.Error("error decoding method", zap.Error(err))
			continue
		}

		if method != 2 {
			h.logger.Error("invalid method", zap.Int64("method", int64(method)))
			continue
		}

		sre, err := decoder.DecodeBytes()
		if err != nil {
			h.logger.Error("error decoding sre", zap.Error(err))
			continue
		}

		// Listen for updates on the registry entry and send updates via WebSocket
		off, err := h.getNode().Services().Registry().Listen(sre, func(entry libs5protocol.SignedRegistryEntry) {
			encoded, err := msgpack.Marshal(entry)
			if err != nil {
				h.logger.Error("error encoding entry", zap.Error(err))
				return
			}

			// Write updates to the WebSocket connection
			if err := c.Write(ctx, websocket.MessageBinary, encoded); err != nil {
				h.logger.Error("error writing to websocket", zap.Error(err))
			}
		})
		if err != nil {
			h.logger.Error("error setting up listener for registry", zap.Error(err))
			break
		}

		listeners = append(listeners, off) // Add the listener's cleanup function to the list
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

func (h *HttpHandler) sendErrorResponse(jc jape.Context, err error) {
	var statusCode int

	switch e := err.(type) {
	case *S5Error:
		statusCode = e.HttpStatus()
	case *account.AccountError:
		mappedCode, ok := account.ErrorCodeToHttpStatus[e.Key]
		if !ok {
			statusCode = http.StatusInternalServerError
		} else {
			statusCode = mappedCode
		}
	default:
		statusCode = http.StatusInternalServerError
		err = errors.New("An internal server error occurred.")
	}

	_ = jc.Error(err, statusCode)
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

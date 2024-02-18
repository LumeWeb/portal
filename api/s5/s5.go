package s5

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.lumeweb.com/LumeWeb/portal/api/swagger"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"git.lumeweb.com/LumeWeb/portal/storage"

	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	s5libmetadata "git.lumeweb.com/LumeWeb/libs5-go/metadata"
	"git.lumeweb.com/LumeWeb/libs5-go/node"
	"git.lumeweb.com/LumeWeb/libs5-go/protocol"
	"git.lumeweb.com/LumeWeb/libs5-go/service"
	storage2 "git.lumeweb.com/LumeWeb/libs5-go/storage"
	"git.lumeweb.com/LumeWeb/libs5-go/storage/provider"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"github.com/samber/lo"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"nhooyr.io/websocket"

	"github.com/julienschmidt/httprouter"

	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	protoRegistry "git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"git.lumeweb.com/LumeWeb/portal/protocols/s5"
	"github.com/rs/cors"
	"github.com/spf13/viper"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API = (*S5API)(nil)
)

//go:embed swagger.yaml
var swagSpec []byte

type S5API struct {
	config     *viper.Viper
	identity   ed25519.PrivateKey
	accounts   *account.AccountServiceDefault
	storage    storage.StorageService
	metadata   metadata.MetadataService
	db         *gorm.DB
	protocols  []protoRegistry.Protocol
	protocol   *s5.S5Protocol
	logger     *zap.Logger
	tusHandler *s5.TusHandler
}

type APIParams struct {
	fx.In
	Config     *viper.Viper
	Identity   ed25519.PrivateKey
	Accounts   *account.AccountServiceDefault
	Storage    storage.StorageService
	Metadata   metadata.MetadataService
	Db         *gorm.DB
	Protocols  []protoRegistry.Protocol `group:"protocol"`
	Logger     *zap.Logger
	TusHandler *s5.TusHandler
}

type S5ApiResult struct {
	fx.Out
	API   registry.API `group:"api"`
	S5API *S5API
}

func NewS5(params APIParams) (S5ApiResult, error) {
	api := &S5API{
		config:     params.Config,
		identity:   params.Identity,
		accounts:   params.Accounts,
		storage:    params.Storage,
		metadata:   params.Metadata,
		db:         params.Db,
		protocols:  params.Protocols,
		logger:     params.Logger,
		tusHandler: params.TusHandler,
	}
	return S5ApiResult{
		API:   api,
		S5API: api,
	}, nil
}

var Module = fx.Module("s5_api",
	fx.Provide(NewS5),
)

func (s *S5API) Init() error {
	s5protocol := protoRegistry.FindProtocolByName("s5", s.protocols)
	if s5protocol == nil {
		return fmt.Errorf("s5 protocol not found")
	}

	s5protocolInstance := s5protocol.(*s5.S5Protocol)
	s.protocol = s5protocolInstance

	return nil
}

func (s S5API) Name() string {
	return "s5"
}

func (s S5API) Start(ctx context.Context) error {
	return s.protocol.Node().Start()
}

func (s S5API) Stop(ctx context.Context) error {
	return nil
}

func (s *S5API) Routes() (*httprouter.Router, error) {
	authMiddlewareOpts := middleware.AuthMiddlewareOptions{
		Identity: s.identity,
		Accounts: s.accounts,
		Config:   s.config,
		Purpose:  account.JWTPurposeLogin,
	}

	authMw := authMiddleware(authMiddlewareOpts)

	tusHandler := BuildS5TusApi(authMw, s.tusHandler)

	tusOptionsHandler := func(c jape.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	}

	tusCors := BuildTusCors()

	wrappedTusHandler := middleware.ApplyMiddlewares(tusOptionsHandler, tusCors, authMw)

	routes := map[string]jape.Handler{
		// Account API
		"GET /s5/account/register":  s.accountRegisterChallenge,
		"POST /s5/account/register": s.accountRegister,
		"GET /s5/account/login":     s.accountLoginChallenge,
		"POST /s5/account/login":    s.accountLogin,
		"GET /s5/account":           middleware.ApplyMiddlewares(s.accountInfo, authMw),
		"GET /s5/account/stats":     middleware.ApplyMiddlewares(s.accountStats, authMw),
		"GET /s5/account/pins.bin":  middleware.ApplyMiddlewares(s.accountPins, authMw),

		// Upload API
		"POST /s5/upload":           middleware.ApplyMiddlewares(s.smallFileUpload, authMw),
		"POST /s5/upload/directory": middleware.ApplyMiddlewares(s.directoryUpload, authMw),

		// Tus API
		"POST /s5/upload/tus":        tusHandler,
		"OPTIONS /s5/upload/tus":     wrappedTusHandler,
		"HEAD /s5/upload/tus/:id":    tusHandler,
		"POST /s5/upload/tus/:id":    tusHandler,
		"PATCH /s5/upload/tus/:id":   tusHandler,
		"OPTIONS /s5/upload/tus/:id": wrappedTusHandler,

		// Download API
		"GET /s5/blob/:cid":     middleware.ApplyMiddlewares(s.downloadBlob, authMw),
		"GET /s5/metadata/:cid": s.downloadMetadata,
		"GET /s5/download/:cid": middleware.ApplyMiddlewares(s.downloadFile, cors.Default().Handler),

		// Pins API
		"POST /s5/pin/:cid":      middleware.ApplyMiddlewares(s.accountPin, authMw),
		"DELETE /s5/delete/:cid": middleware.ApplyMiddlewares(s.accountPinDelete, authMw),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      middleware.ApplyMiddlewares(s.debugDownloadUrls, authMw),
		"GET /s5/debug/storage_locations/:hash": middleware.ApplyMiddlewares(s.debugStorageLocations, authMw),

		// Registry API
		"GET /s5/registry":              middleware.ApplyMiddlewares(s.registryQuery, authMw),
		"POST /s5/registry":             middleware.ApplyMiddlewares(s.registrySet, authMw),
		"GET /s5/registry/subscription": middleware.ApplyMiddlewares(s.registrySubscription, authMw),
	}

	routes, err := swagger.Swagger(swagSpec, routes)
	if err != nil {
		return nil, err
	}

	return s.protocol.Node().Services().HTTP().GetHttpRouter(routes), nil
}

type s5TusJwtResponseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (w *s5TusJwtResponseWriter) WriteHeader(statusCode int) {
	// Check if this is the specific route and status
	if statusCode == http.StatusCreated {
		location := w.Header().Get("Location")
		authToken := middleware.ParseAuthTokenHeader(w.req.Header)

		if authToken != "" && location != "" {

			parsedUrl, _ := url.Parse(location)

			query := parsedUrl.Query()
			query.Set("auth_token", authToken)
			parsedUrl.RawQuery = query.Encode()

			w.Header().Set("Location", parsedUrl.String())
		}
	}

	w.ResponseWriter.WriteHeader(statusCode)
}

func BuildTusCors() func(h http.Handler) http.Handler {
	mw :=
		cors.New(cors.Options{
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders: []string{
				"Authorization",
				"Expires",
				"Upload-Concat",
				"Upload-Length",
				"Upload-Offset",
				"X-Requested-With",
				"Tus-Version",
				"Tus-Resumable",
				"Tus-Extension",
				"Tus-Max-Size",
				"X-HTTP-Method-Override",
			},
			AllowCredentials: true,
		})

	return mw.Handler
}

func BuildS5TusApi(authMw middleware.HttpMiddlewareFunc, handler *s5.TusHandler) jape.Handler {
	// Create a jape.Handler for your tusHandler
	tusJapeHandler := func(c jape.Context) {
		tusHandler := handler.Tus()
		tusHandler.ServeHTTP(c.ResponseWriter, c.Request)
	}

	protocolMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "protocol", "s5")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	stripPrefix := func(next http.Handler) http.Handler {
		return http.StripPrefix("/s5/upload/tus", next)
	}

	injectJwt := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			res := w
			if r.Method == http.MethodPost && r.URL.Path == "/s5/upload/tus" {
				res = &s5TusJwtResponseWriter{ResponseWriter: w, req: r}
			}

			next.ServeHTTP(res, r)
		})
	}

	// Apply the middlewares to the tusJapeHandler
	tusHandler := middleware.ApplyMiddlewares(tusJapeHandler, BuildTusCors(), authMw, injectJwt, protocolMiddleware, stripPrefix, middleware.ProxyMiddleware)

	return tusHandler
}

type readSeekNopCloser struct {
	*bytes.Reader
}

func (rsnc readSeekNopCloser) Close() error {
	return nil
}

func (s *S5API) smallFileUpload(jc jape.Context) {
	user := middleware.GetUserFromContext(jc.Request.Context())

	file, err := s.prepareFileUpload(jc)
	if err != nil {
		s.sendErrorResponse(jc, err)
		return
	}
	defer func(file io.ReadSeekCloser) {
		err := file.Close()
		if err != nil {
			s.logger.Error("Error closing file", zap.Error(err))
		}
	}(file)

	newUpload, err2 := s.storage.UploadObject(jc.Request.Context(), s5.GetStorageProtocol(s.protocol), file, nil, nil)

	if err2 != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	newUpload.UserID = user
	newUpload.UploaderIP = jc.Request.RemoteAddr

	err2 = s.metadata.SaveUpload(jc.Request.Context(), *newUpload)

	if err2 != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	cid, err2 := encoding.CIDFromHash(newUpload.Hash, newUpload.Size, types.CIDTypeRaw, types.HashTypeBlake3)
	if err2 != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	cidStr, err2 := cid.ToString()
	if err2 != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyFileUploadFailed, err2))
		return
	}

	jc.Encode(&SmallUploadResponse{
		CID: cidStr,
	})
}

func (s *S5API) prepareFileUpload(jc jape.Context) (file io.ReadSeekCloser, s5Err *S5Error) {
	r := jc.Request
	contentType := r.Header.Get("Content-Type")

	// Handle multipart form data uploads
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := r.ParseMultipartForm(s.config.GetInt64("core.post-upload-limit")); err != nil {
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

func (s *S5API) accountRegisterChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	challenge := make([]byte, 32)
	_, err := rand.Read(challenge)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	if len(decodedKey) != 33 || int(decodedKey[0]) != int(types.HashTypeEd25519) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyDataIntegrityError, fmt.Errorf("invalid public key format")))
		return
	}

	result := s.db.Create(&models.S5Challenge{
		Pubkey:    pubkey,
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "register",
	})

	if result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	jc.Encode(&AccountRegisterChallengeResponse{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
	})
}

func (s *S5API) accountRegister(jc jape.Context) {
	var request AccountRegisterRequest
	if jc.Decode(&request) != nil {
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(request.Pubkey)
	if err != nil || len(decodedKey) != 33 || int(decodedKey[0]) != int(types.HashTypeEd25519) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	challenge := models.S5Challenge{
		Pubkey: request.Pubkey,
		Type:   "register",
	}

	if result := s.db.Where(&challenge).First(&challenge); result.RowsAffected == 0 || result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, result.Error))
		return
	}

	decodedResponse, err := base64.RawURLEncoding.DecodeString(request.Response)
	if err != nil || len(decodedResponse) != 65 {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyDataIntegrityError, err))
		return
	}

	decodedChallenge, err := base64.RawURLEncoding.DecodeString(challenge.Challenge)
	if err != nil || !bytes.Equal(decodedResponse[1:33], decodedChallenge) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil || !ed25519.Verify(decodedKey[1:], decodedResponse, decodedSignature) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyAuthorizationFailed, err))
		return
	}

	if request.Email == "" {
		request.Email = fmt.Sprintf("%s@%s", hex.EncodeToString(decodedKey[1:]), "example.com")
	}

	if accountExists, _, _ := s.accounts.EmailExists(request.Email); accountExists {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyResourceLimitExceeded, fmt.Errorf("email already exists")))
		return
	}

	if pubkeyExists, _, _ := s.accounts.PubkeyExists(hex.EncodeToString(decodedKey[1:])); pubkeyExists {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyResourceLimitExceeded, fmt.Errorf("pubkey already exists")))
		return
	}

	passwd := make([]byte, 32)
	if _, err = rand.Read(passwd); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
		return
	}

	newAccount, err := s.accounts.CreateAccount(request.Email, string(passwd))
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	rawPubkey := hex.EncodeToString(decodedKey[1:])
	if err = s.accounts.AddPubkeyToAccount(*newAccount, rawPubkey); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jwt, err := s.accounts.LoginPubkey(rawPubkey)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyAuthenticationFailed, err))
		return
	}

	if result := s.db.Delete(&challenge); result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	setAuthCookie(jwt, jc)
}

func (s *S5API) accountLoginChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	challenge := make([]byte, 32)
	_, err := rand.Read(challenge)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(pubkey)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	if len(decodedKey) != 33 || int(decodedKey[0]) != int(types.HashTypeEd25519) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyUnsupportedFileType, fmt.Errorf("public key not supported")))
		return
	}

	pubkeyExists, _, _ := s.accounts.PubkeyExists(hex.EncodeToString(decodedKey[1:]))
	if !pubkeyExists {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, fmt.Errorf("public key does not exist")))
		return
	}

	result := s.db.Create(&models.S5Challenge{
		Pubkey:    pubkey,
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "login",
	})

	if result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	jc.Encode(&AccountLoginChallengeResponse{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
	})
}

func (s *S5API) accountLogin(jc jape.Context) {
	var request AccountLoginRequest
	if jc.Decode(&request) != nil {
		return
	}

	decodedKey, err := base64.RawURLEncoding.DecodeString(request.Pubkey)
	if err != nil || len(decodedKey) != 32 {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err))
		return
	}

	if int(decodedKey[0]) != int(types.HashTypeEd25519) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyUnsupportedFileType, fmt.Errorf("public key type not supported")))
		return
	}

	var challenge models.S5Challenge
	result := s.db.Where(&models.S5Challenge{Pubkey: request.Pubkey, Type: "login"}).First(&challenge)
	if result.RowsAffected == 0 || result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, result.Error))
		return
	}

	decodedResponse, err := base64.RawURLEncoding.DecodeString(request.Response)
	if err != nil || len(decodedResponse) != 65 {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	decodedChallenge, err := base64.RawURLEncoding.DecodeString(challenge.Challenge)
	if err != nil || !bytes.Equal(decodedResponse[1:33], decodedChallenge) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyDataIntegrityError, err))
		return
	}

	decodedSignature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil || !ed25519.Verify(decodedKey[1:], decodedResponse, decodedSignature) {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyAuthorizationFailed, err))
		return
	}

	jwt, err := s.accounts.LoginPubkey(hex.EncodeToString(decodedKey[1:])) // Adjust based on how LoginPubkey is implemented
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyAuthenticationFailed, err))
		return
	}

	if result := s.db.Delete(&challenge); result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	setAuthCookie(jwt, jc)
}

func (s *S5API) accountInfo(jc jape.Context) {
	userID := middleware.GetUserFromContext(jc.Request.Context())
	_, user, _ := s.accounts.AccountExists(userID)

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

func (s *S5API) accountStats(jc jape.Context) {
	userID := middleware.GetUserFromContext(jc.Request.Context())
	_, user, _ := s.accounts.AccountExists(userID)

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

func (s *S5API) accountPins(jc jape.Context) {
	var cursor uint64
	if err := jc.DecodeForm("cursor", &cursor); err != nil {
		return
	}

	userID := middleware.GetUserFromContext(jc.Request.Context())

	pins, err := s.accounts.AccountPins(userID, cursor)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	pinResponse := &AccountPinResponse{Cursor: cursor, Pins: pins}
	result, err2 := msgpack.Marshal(pinResponse)
	if err2 != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err2))
		return
	}

	jc.ResponseWriter.Header().Set("Content-Type", "application/msgpack")
	jc.ResponseWriter.WriteHeader(http.StatusOK)
	if _, err := jc.ResponseWriter.Write(result); err != nil {
		s.logger.Error("failed to write account pins response", zap.Error(err))
	}
}

func (s *S5API) accountPinDelete(jc jape.Context) {
	var cid string
	if err := jc.DecodeParam("cid", &cid); err != nil {
		return
	}

	user := middleware.GetUserFromContext(jc.Request.Context())

	decodedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	if err := s.accounts.DeletePinByHash(decodedCid.Hash.HashBytes(), user); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (s *S5API) accountPin(jc jape.Context) {
	var cid string
	if err := jc.DecodeParam("cid", &cid); err != nil {
		return
	}

	userID := middleware.GetUserFromContext(jc.Request.Context())

	decodedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	if err := s.accounts.PinByHash(decodedCid.Hash.HashBytes(), userID); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (s *S5API) directoryUpload(jc jape.Context) {
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
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, fmt.Errorf("expected multipart/form-data content type, got %s", contentType)))
		return
	}

	// Parse multipart form with size limit from config
	if err := jc.Request.ParseMultipartForm(s.config.GetInt64("core.post-upload-limit")); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	uploads, err := s.processMultipartFiles(jc.Request)
	if err != nil {
		s.sendErrorResponse(jc, err)
		return
	}

	// Generate metadata for the directory upload
	app, err := s.createAppMetadata(name, tryFiles, errorPages, uploads)
	if err != nil {
		s.sendErrorResponse(jc, err)
		return
	}

	// Upload the metadata
	cidStr, err := s.uploadAppMetadata(app, jc.Request)
	if err != nil {
		s.sendErrorResponse(jc, err)
		return
	}

	jc.Encode(&AppUploadResponse{CID: cidStr})
}

func (s *S5API) processMultipartFiles(r *http.Request) (map[string]*metadata.UploadMetadata, error) {
	uploadMap := make(map[string]*metadata.UploadMetadata)
	user := middleware.GetUserFromContext(r.Context())

	for _, files := range r.MultipartForm.File {
		for _, fileHeader := range files {
			file, err := fileHeader.Open()
			if err != nil {
				return nil, NewS5Error(ErrKeyStorageOperationFailed, err)
			}
			defer func(file multipart.File) {
				err := file.Close()
				if err != nil {
					s.logger.Error("Error closing file", zap.Error(err))
				}
			}(file)

			upload, err := s.storage.UploadObject(r.Context(), s5.GetStorageProtocol(s.protocol), file, nil, nil)
			if err != nil {
				return nil, NewS5Error(ErrKeyStorageOperationFailed, err)
			}

			upload.UserID = user
			upload.UploaderIP = r.RemoteAddr

			err = s.metadata.SaveUpload(r.Context(), *upload)
			if err != nil {
				return nil, NewS5Error(ErrKeyStorageOperationFailed, err)
			}

			uploadMap[fileHeader.Filename] = upload
		}
	}

	return uploadMap, nil
}

func (s *S5API) createAppMetadata(name string, tryFiles []string, errorPages map[int]string, uploads map[string]*metadata.UploadMetadata) (*s5libmetadata.WebAppMetadata, error) {
	filesMap := make(map[string]s5libmetadata.WebAppMetadataFileReference, len(uploads))

	for filename, upload := range uploads {
		hash := upload.Hash

		cid, err := encoding.CIDFromHash(hash, upload.Size, types.CIDTypeRaw, types.HashTypeBlake3)
		if err != nil {
			return nil, NewS5Error(ErrKeyInternalError, err, "Failed to create CID for file: "+filename)
		}
		filesMap[filename] = s5libmetadata.WebAppMetadataFileReference{
			Cid:         cid,
			ContentType: upload.MimeType,
		}
	}

	extraMetadataMap := make(map[int]interface{})
	for statusCode, page := range errorPages {
		extraMetadataMap[statusCode] = page
	}

	extraMetadata := s5libmetadata.NewExtraMetadata(extraMetadataMap)
	// Create the web app metadata object
	app := s5libmetadata.NewWebAppMetadata(
		name,
		tryFiles,
		*extraMetadata,
		errorPages,
		filesMap,
	)

	return app, nil
}

func (s *S5API) uploadAppMetadata(appData *s5libmetadata.WebAppMetadata, r *http.Request) (string, *S5Error) {
	userId := middleware.GetUserFromContext(r.Context())

	appDataRaw, err := msgpack.Marshal(appData)
	if err != nil {
		return "", NewS5Error(ErrKeyInternalError, err, "Failed to marshal app s5libmetadata")
	}

	file := bytes.NewReader(appDataRaw)

	upload, err := s.storage.UploadObject(r.Context(), s5.GetStorageProtocol(s.protocol), file, nil, nil)
	if err != nil {
		return "", NewS5Error(ErrKeyStorageOperationFailed, err)
	}

	upload.UserID = userId
	upload.UploaderIP = r.RemoteAddr

	err = s.metadata.SaveUpload(r.Context(), *upload)
	if err != nil {
		return "", NewS5Error(ErrKeyStorageOperationFailed, err)
	}

	// Construct the CID for the newly uploaded s5libmetadata
	cid, err := encoding.CIDFromHash(upload.Hash, uint64(len(appDataRaw)), types.CIDTypeMetadataWebapp, types.HashTypeBlake3)
	if err != nil {
		return "", NewS5Error(ErrKeyInternalError, err, "Failed to create CID for new app s5libmetadata")
	}
	cidStr, err := cid.ToString()
	if err != nil {
		return "", NewS5Error(ErrKeyInternalError, err, "Failed to convert CID to string for new app s5libmetadata")
	}

	return cidStr, nil
}

func (s *S5API) debugDownloadUrls(jc jape.Context) {
	var cid string
	if err := jc.DecodeParam("cid", &cid); err != nil {
		return
	}

	decodedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err, "Failed to decode CID"))
		return
	}

	node := s.getNode()
	dlUriProvider := s.newStorageLocationProvider(&decodedCid.Hash, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

	if err := dlUriProvider.Start(); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Failed to start URI provider"))
		return
	}

	locations, err := node.Services().Storage().GetCachedStorageLocations(&decodedCid.Hash, []types.StorageLocationType{
		types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge,
	})
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Failed to get cached storage locations"))
		return
	}

	availableNodes := lo.Keys[string, storage2.StorageLocation](locations)
	availableNodesIds := make([]*encoding.NodeId, len(availableNodes))

	for i, nodeIdStr := range availableNodes {
		nodeId, err := encoding.DecodeNodeId(nodeIdStr)
		if err != nil {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err, "Failed to decode node ID"))
			return
		}
		availableNodesIds[i] = nodeId
	}

	sorted, err := node.Services().P2P().SortNodesByScore(availableNodesIds)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyNetworkError, err, "Failed to sort nodes by score"))
		return
	}

	output := make([]string, len(sorted))
	for i, nodeId := range sorted {
		nodeIdStr, err := nodeId.ToString()
		if err != nil {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err, "Failed to convert node ID to string"))
			return
		}
		output[i] = locations[nodeIdStr].BytesURL()
	}

	jc.ResponseWriter.WriteHeader(http.StatusOK)
	_, err = jc.ResponseWriter.Write([]byte(strings.Join(output, "\n")))
	if err != nil {
		s.logger.Error("Failed to write response", zap.Error(err))
	}
}

func (s *S5API) registryQuery(jc jape.Context) {
	var pk string
	if err := jc.DecodeForm("pk", &pk); err != nil {
		return
	}

	pkBytes, err := base64.RawURLEncoding.DecodeString(pk)
	if err != nil {
		s5Err := NewS5Error(ErrKeyInvalidFileFormat, err)
		s.sendErrorResponse(jc, s5Err)
		return
	}

	entry, err := s.getNode().Services().Registry().Get(pkBytes)
	if err != nil {
		s5ErrKey := ErrKeyStorageOperationFailed
		s5Err := NewS5Error(s5ErrKey, err)
		s.sendErrorResponse(jc, s5Err)
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

func (s *S5API) registrySet(jc jape.Context) {
	var request RegistrySetRequest

	if err := jc.Decode(&request); err != nil {
		return
	}

	pk, err := base64.RawURLEncoding.DecodeString(request.Pk)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err, "Error decoding public key"))
		return
	}

	data, err := base64.RawURLEncoding.DecodeString(request.Data)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err, "Error decoding data"))
		return
	}

	signature, err := base64.RawURLEncoding.DecodeString(request.Signature)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidFileFormat, err, "Error decoding signature"))
		return
	}

	entry := protocol.NewSignedRegistryEntry(pk, request.Revision, data, signature)

	err = s.getNode().Services().Registry().Set(entry, false, nil)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Error setting registry entry"))
		return
	}
}
func (s *S5API) registrySubscription(jc jape.Context) {
	// Create a context for the WebSocket operations
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var listeners []func()

	// Accept the WebSocket connection
	c, err := websocket.Accept(jc.ResponseWriter, jc.Request, nil)
	if err != nil {
		s.logger.Error("error accepting websocket connection", zap.Error(err))
		return
	}
	defer func() {
		// Close the WebSocket connection gracefully
		err := c.Close(websocket.StatusNormalClosure, "connection closed")
		if err != nil {
			s.logger.Error("error closing websocket connection", zap.Error(err))
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
				s.logger.Info("websocket connection closed normally")
			} else {
				// Handle different types of errors
				s.logger.Error("error in websocket connection", zap.Error(err))
			}
			break
		}

		decoder := msgpack.NewDecoder(bytes.NewReader(data))

		// Assuming method indicates the type of operation, validate it
		method, err := decoder.DecodeInt()
		if err != nil {
			s.logger.Error("error decoding method", zap.Error(err))
			continue
		}

		if method != 2 {
			s.logger.Error("invalid method", zap.Int64("method", int64(method)))
			continue
		}

		sre, err := decoder.DecodeBytes()
		if err != nil {
			s.logger.Error("error decoding sre", zap.Error(err))
			continue
		}

		// Listen for updates on the registry entry and send updates via WebSocket
		off, err := s.getNode().Services().Registry().Listen(sre, func(entry protocol.SignedRegistryEntry) {
			encoded, err := msgpack.Marshal(entry)
			if err != nil {
				s.logger.Error("error encoding entry", zap.Error(err))
				return
			}

			// Write updates to the WebSocket connection
			if err := c.Write(ctx, websocket.MessageBinary, encoded); err != nil {
				s.logger.Error("error writing to websocket", zap.Error(err))
			}
		})
		if err != nil {
			s.logger.Error("error setting up listener for registry", zap.Error(err))
			break
		}

		listeners = append(listeners, off) // Add the listener's cleanup function to the list
	}
}

func (s *S5API) getNode() *node.Node {
	return s.protocol.Node()
}

func (s *S5API) downloadBlob(jc jape.Context) {
	var cid string

	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	cid = strings.Split(cid, ".")[0]

	cidDecoded, err := encoding.CIDFromString(cid)
	if jc.Check("error decoding cid", err) != nil {
		return
	}

	dlUriProvider := s.newStorageLocationProvider(&cidDecoded.Hash, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

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

func (s *S5API) debugStorageLocations(jc jape.Context) {
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

	dlUriProvider := s.newStorageLocationProvider(decodedHash, typeIntList...)

	err = dlUriProvider.Start()
	if jc.Check("error starting search", err) != nil {
		return
	}

	_, err = dlUriProvider.Next()
	if jc.Check("error fetching locations", err) != nil {
		return
	}

	locations, err := s.getNode().Services().Storage().GetCachedStorageLocations(decodedHash, typeIntList)
	if jc.Check("error getting cached locations", err) != nil {
		return
	}

	availableNodes := lo.Keys[string, storage2.StorageLocation](locations)
	availableNodesIds := make([]*encoding.NodeId, len(availableNodes))

	for i, nodeIdStr := range availableNodes {
		nodeId, err := encoding.DecodeNodeId(nodeIdStr)
		if jc.Check("error decoding node id", err) != nil {
			return
		}
		availableNodesIds[i] = nodeId
	}

	availableNodesIds, err = s.getNode().Services().P2P().SortNodesByScore(availableNodesIds)

	if jc.Check("error sorting nodes", err) != nil {
		return
	}

	debugLocations := make([]DebugStorageLocation, len(availableNodes))

	for i, nodeId := range availableNodesIds {
		nodeIdStr, err := nodeId.ToBase58()
		if jc.Check("error encoding node id", err) != nil {
			return
		}

		score, err := s.getNode().Services().P2P().GetNodeScore(nodeId)

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

func (s *S5API) downloadMetadata(jc jape.Context) {
	var cid string

	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	cidDecoded, err := encoding.CIDFromString(cid)
	if jc.Check("error decoding cid", err) != nil {
		s.logger.Error("error decoding cid", zap.Error(err))
		return
	}

	switch cidDecoded.Type {
	case types.CIDTypeRaw:
		_ = jc.Error(errors.New("Raw CIDs do not have s5libmetadata"), http.StatusBadRequest)
		return

	case types.CIDTypeResolver:
		_ = jc.Error(errors.New("Resolver CIDs not yet supported"), http.StatusBadRequest)
		return
	}

	meta, err := s.getNode().Services().Storage().GetMetadataByCID(cidDecoded)

	if jc.Check("error getting s5libmetadata", err) != nil {
		s.logger.Error("error getting s5libmetadata", zap.Error(err))
		return
	}

	if cidDecoded.Type != types.CIDTypeBridge {
		jc.ResponseWriter.Header().Set("Cache-Control", "public, max-age=31536000")
	} else {
		jc.ResponseWriter.Header().Set("Cache-Control", "public, max-age=60")
	}

	jc.Encode(&meta)

}

func (s *S5API) downloadFile(jc jape.Context) {
	var cid string

	if jc.DecodeParam("cid", &cid) != nil {
		return
	}

	var hashBytes []byte
	isProof := false

	if strings.HasSuffix(cid, storage.PROOF_EXTENSION) {
		isProof = true
		cid = strings.TrimSuffix(cid, storage.PROOF_EXTENSION)
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

	file := s.newFile(s.protocol, hashBytes)

	if !file.Exists() {
		jc.ResponseWriter.WriteHeader(http.StatusNotFound)
		return
	}

	defer func(file io.ReadCloser) {
		err := file.Close()
		if err != nil {
			s.logger.Error("error closing file", zap.Error(err))
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

func (s *S5API) sendErrorResponse(jc jape.Context, err error) {
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

func (s *S5API) newStorageLocationProvider(hash *encoding.Multihash, types ...types.StorageLocationType) storage2.StorageLocationProvider {
	return provider.NewStorageLocationProvider(provider.StorageLocationProviderParams{
		Services:      s.getNode().Services(),
		Hash:          hash,
		LocationTypes: types,
		ServiceParams: service.ServiceParams{
			Logger: s.logger,
			Config: s.getNode().Config(),
			Db:     s.getNode().Db(),
		},
	})
}

func (s *S5API) newFile(protocol *s5.S5Protocol, hash []byte) *S5File {
	return NewFile(FileParams{
		Protocol: protocol,
		Hash:     hash,
		Metadata: s.metadata,
		Storage:  s.storage,
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

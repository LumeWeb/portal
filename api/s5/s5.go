package s5

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"git.lumeweb.com/LumeWeb/portal/api/router"
	"git.lumeweb.com/LumeWeb/portal/bao"
	"git.lumeweb.com/LumeWeb/portal/renter"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"git.lumeweb.com/LumeWeb/portal/cron"

	"git.lumeweb.com/LumeWeb/portal/config"

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
	"github.com/ddo/rq"
	dnslink "github.com/dnslink-std/go"
	"github.com/golang-queue/queue"
	"github.com/rs/cors"
	"go.sia.tech/jape"
	"go.uber.org/fx"
)

var (
	_ registry.API       = (*S5API)(nil)
	_ router.RoutableAPI = (*S5API)(nil)
)

//go:embed swagger.yaml
var swagSpec []byte

type S5API struct {
	config     *config.Manager
	identity   ed25519.PrivateKey
	accounts   *account.AccountServiceDefault
	storage    storage.StorageService
	metadata   metadata.MetadataService
	db         *gorm.DB
	protocols  []protoRegistry.Protocol
	protocol   *s5.S5Protocol
	logger     *zap.Logger
	tusHandler *s5.TusHandler
	cron       *cron.CronServiceDefault
}

type APIParams struct {
	fx.In
	Config     *config.Manager
	Identity   ed25519.PrivateKey
	Accounts   *account.AccountServiceDefault
	Storage    storage.StorageService
	Metadata   metadata.MetadataService
	Db         *gorm.DB
	Protocols  []protoRegistry.Protocol `group:"protocol"`
	Logger     *zap.Logger
	TusHandler *s5.TusHandler
	Cron       *cron.CronServiceDefault
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
		cron:       params.Cron,
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
	return s.protocol.Node().Start(ctx)
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

	corsOptionsHandler := func(c jape.Context) {
		c.ResponseWriter.WriteHeader(http.StatusOK)
	}

	tusCors := BuildTusCors()

	wrappedTusHandler := middleware.ApplyMiddlewares(corsOptionsHandler, tusCors, authMw)

	debugCors := cors.Default()

	defaultCors := cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool {
			return true
		},
		AllowedMethods:   []string{"POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})

	routes := map[string]jape.Handler{
		// Account API
		"GET /s5/account/register":  s.accountRegisterChallenge,
		"POST /s5/account/register": s.accountRegister,
		"GET /s5/account/login":     s.accountLoginChallenge,
		"POST /s5/account/login":    s.accountLogin,
		"GET /s5/account":           middleware.ApplyMiddlewares(s.accountInfo, authMw),
		"GET /s5/account/stats":     middleware.ApplyMiddlewares(s.accountStats, authMw),
		"GET /s5/account/pins.bin":  middleware.ApplyMiddlewares(s.accountPinsBinary, authMw),
		"GET /s5/account/pins":      middleware.ApplyMiddlewares(s.accountPins, authMw),

		// Upload API
		"POST /s5/upload":              middleware.ApplyMiddlewares(s.smallFileUpload, defaultCors.Handler, authMw),
		"POST /s5/upload/directory":    middleware.ApplyMiddlewares(s.directoryUpload, defaultCors.Handler, authMw),
		"OPTIONS /s5/upload":           middleware.ApplyMiddlewares(corsOptionsHandler, defaultCors.Handler, authMw),
		"OPTIONS /s5/upload/directory": middleware.ApplyMiddlewares(corsOptionsHandler, defaultCors.Handler, authMw),

		// Tus API
		"POST /s5/upload/tus":        tusHandler,
		"OPTIONS /s5/upload/tus":     wrappedTusHandler,
		"HEAD /s5/upload/tus/:id":    tusHandler,
		"POST /s5/upload/tus/:id":    tusHandler,
		"PATCH /s5/upload/tus/:id":   tusHandler,
		"OPTIONS /s5/upload/tus/:id": wrappedTusHandler,

		// Download API
		"GET /s5/blob/:cid":         middleware.ApplyMiddlewares(s.downloadBlob, authMw),
		"GET /s5/metadata/:cid":     s.downloadMetadata,
		"GET /s5/download/:cid":     middleware.ApplyMiddlewares(s.downloadFile, defaultCors.Handler),
		"OPTIONS /s5/blob/:cid":     middleware.ApplyMiddlewares(corsOptionsHandler, defaultCors.Handler, authMw),
		"OPTIONS /s5/metadata/:cid": middleware.ApplyMiddlewares(corsOptionsHandler, defaultCors.Handler),
		"OPTIONS /s5/download/:cid": middleware.ApplyMiddlewares(corsOptionsHandler, defaultCors.Handler),

		// Pins API
		"POST /s5/pin/:cid":      middleware.ApplyMiddlewares(s.accountPin, authMw),
		"DELETE /s5/delete/:cid": middleware.ApplyMiddlewares(s.accountPinDelete, authMw),

		// Debug API
		"GET /s5/debug/download_urls/:cid":      middleware.ApplyMiddlewares(s.debugDownloadUrls, middleware.ProxyMiddleware, debugCors.Handler),
		"GET /s5/debug/storage_locations/:hash": middleware.ApplyMiddlewares(s.debugStorageLocations, middleware.ProxyMiddleware, debugCors.Handler),

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

func (s *S5API) Can(w http.ResponseWriter, r *http.Request) bool {
	host := r.Host
	if strings.Contains(host, ":") {
		host = strings.Split(host, ":")[0]
	}
	resolve, err := dnslink.Resolve(host)
	if err != nil {
		return false
	}

	if _, ok := resolve.Links[s.Name()]; !ok {
		return false
	}

	decodedCid, err := encoding.CIDFromString(resolve.Links[s.Name()][0].Identifier)

	if err != nil {
		s.logger.Error("Error decoding CID", zap.Error(err))
		return false
	}

	hash := decodedCid.Hash.HashBytes()

	upload, err := s.metadata.GetUpload(r.Context(), hash)
	if err != nil {
		return false
	}

	if upload.Protocol != s.Name() {
		return false
	}

	exists, _, err := s.accounts.DNSLinkExists(hash)
	if err != nil {
		return false
	}

	if !exists {
		return false
	}

	ctx := context.WithValue(r.Context(), "cid", decodedCid)

	*r = *r.WithContext(ctx)

	return true
}

func (s *S5API) Handle(w http.ResponseWriter, r *http.Request) {
	cidVal := r.Context().Value("cid")

	if cidVal == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	cid := cidVal.(*encoding.CID)

	if cid.Type == types.CIDTypeResolver {
		entry, err := s.getNode().Services().Registry().Get(cid.Hash.FullBytes())
		if err != nil {
			s.logger.Error("Error getting registry entry", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		cid, err = encoding.CIDFromRegistry(entry.Data())
		if err != nil {
			s.logger.Error("Error getting CID from registry entry", zap.Error(err))
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}

	switch cid.Type {
	case types.CIDTypeRaw:
		s.handleDnsLinkRaw(w, r, cid)
	case types.CIDTypeMetadataWebapp:
		s.handleDnsLinkWebapp(w, r, cid)
	case types.CIDTypeDirectory:
		s.handleDnsLinkDirectory(w, r, cid)
	default:
		w.WriteHeader(http.StatusUnsupportedMediaType)
	}
}

func (s *S5API) handleDnsLinkRaw(w http.ResponseWriter, r *http.Request, cid *encoding.CID) {
	file := s.newFile(FileParams{
		Hash: cid.Hash.HashBytes(),
		Type: cid.Type,
	})

	if !file.Exists() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	defer func(file io.ReadCloser) {
		err := file.Close()
		if err != nil {
			s.logger.Error("error closing file", zap.Error(err))
		}
	}(file)

	w.Header().Set("Content-Type", file.Mime())

	http.ServeContent(w, r, file.Name(), file.Modtime(), file)
}

func (s *S5API) handleDnsLinkWebapp(w http.ResponseWriter, r *http.Request, cid *encoding.CID) {
	http.FileServer(http.FS(newWebAppFs(cid, s))).ServeHTTP(w, r)
}

func (s *S5API) handleDnsLinkDirectory(w http.ResponseWriter, r *http.Request, cid *encoding.CID) {
	http.FileServer(http.FS(newDirFs(cid, s))).ServeHTTP(w, r)
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
			AllowOriginFunc: func(origin string) bool {
				return true
			},
			AllowedMethods: []string{"GET", "POST", "PATCH", "DELETE", "HEAD", "OPTIONS"},
			AllowedHeaders: []string{
				"Authorization",
				"Expires",
				"Upload-Concat",
				"Upload-Length",
				"Upload-Metadata",
				"Upload-Offset",
				"X-Requested-With",
				"Tus-Version",
				"Tus-Resumable",
				"Tus-Extension",
				"Tus-Max-Size",
				"X-HTTP-Method-Override",
				"Content-Type",
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
		if err := r.ParseMultipartForm(int64(s.config.Config().Core.PostUploadLimit)); err != nil {
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

	newAccount, err := s.accounts.CreateAccount(request.Email, string(passwd), false)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	rawPubkey := hex.EncodeToString(decodedKey[1:])
	if err = s.accounts.AddPubkeyToAccount(*newAccount, rawPubkey); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	jwt, err := s.accounts.LoginPubkey(rawPubkey, jc.Request.RemoteAddr)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyAuthenticationFailed, err))
		return
	}

	if result := s.db.Delete(&challenge); result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	account.SetAuthCookie(jc, jwt, s.Name())
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

	jwt, err := s.accounts.LoginPubkey(hex.EncodeToString(decodedKey[1:]), jc.Request.RemoteAddr)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyAuthenticationFailed, err))
		return
	}

	if result := s.db.Delete(&challenge); result.Error != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, result.Error))
		return
	}

	account.SetAuthCookie(jc, jwt, s.Name())
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

func (s *S5API) accountPinsBinary(jc jape.Context) {
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

	pinResponse := &AccountPinBinaryResponse{Cursor: cursor, Pins: pins}
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

func (s *S5API) accountPins(jc jape.Context) {
	userID := middleware.GetUserFromContext(jc.Request.Context())
	pinsRet, err := s.accounts.AccountPins(userID, 0)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
		return
	}

	pins := make([]AccountPin, len(pinsRet))

	for i, pin := range pinsRet {
		base64Url, err := encoding.NewMultihash(append([]byte{byte(types.HashTypeBlake3)}, pin.Upload.Hash...)).ToBase64Url()
		if err != nil {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
			return
		}
		pins[i] = AccountPin{
			Hash:     base64Url,
			MimeType: pin.Upload.MimeType,
		}
	}

	jc.Encode(&AccountPinResponse{Pins: pins})
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

func (s *S5API) getManifestCids(ctx context.Context, cid *encoding.CID, addSelf bool) ([]*encoding.CID, error) {
	var cids []*encoding.CID

	if middleware.CtxAborted(ctx) {
		return nil, ctx.Err()
	}

	manifest, err := s.getNode().Services().Storage().GetMetadataByCID(cid)
	if err != nil {
		return nil, err
	}

	if addSelf {
		cids = append(cids, cid)
	}

	switch cid.Type {
	case types.CIDTypeMetadataMedia:
		media := manifest.(*s5libmetadata.MediaMetadata)
		for _, mediaType := range media.MediaTypes {
			lo.ForEach(mediaType, func(format s5libmetadata.MediaFormat, _i int) {
				if format.Cid != nil {
					cids = append(cids, format.Cid)
				}
			})
		}

	case types.CIDTypeDirectory:
		dir := manifest.(*s5libmetadata.DirectoryMetadata)

		lo.ForEach(lo.Values(dir.Directories.Items()), func(d *s5libmetadata.DirectoryReference, _i int) {
			if middleware.CtxAborted(ctx) {
				return
			}
			entry, err := s.getNode().Services().Registry().Get(d.PublicKey)
			if err != nil || entry == nil {
				s.logger.Error("Error getting registry entry", zap.Error(err))
				return
			}

			cid, err := encoding.CIDFromRegistry(entry.Data())
			if err != nil {
				s.logger.Error("Error getting CID from registry entry", zap.Error(err))
				return
			}

			childCids, err := s.getManifestCids(ctx, cid, true)
			if err != nil {
				s.logger.Error("Error getting child manifest CIDs", zap.Error(err))
				return
			}

			cids = append(cids, childCids...)
		})

		lo.ForEach(lo.Values(dir.Files.Items()), func(f *s5libmetadata.FileReference, _i int) {
			cids = append(cids, f.File.CID())
		})

	case types.CIDTypeMetadataWebapp:
		webapp := manifest.(*s5libmetadata.WebAppMetadata)

		lo.ForEach(webapp.Paths.Values(), func(f s5libmetadata.WebAppMetadataFileReference, _i int) {
			cids = append(cids, f.Cid)
		})
	}

	if middleware.CtxAborted(ctx) {
		return nil, ctx.Err()
	}

	return cids, nil
}

func (s *S5API) accountPinManifest(jc jape.Context, userId uint, cid *encoding.CID, addSelf bool) {
	type pinResult struct {
		Success bool  `json:"success"`
		Error   error `json:"error,omitempty"`
	}

	type pinQueueResult struct {
		success bool
		error   error
		cid     *encoding.CID
	}

	cids, err := s.getManifestCids(jc.Request.Context(), cid, addSelf)
	if err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	q := queue.NewPool(10)
	defer q.Release()
	rets := make(chan pinQueueResult)
	defer close(rets)

	results := make(map[string]pinResult, len(cids))

	for i := 0; i < len(cids); i++ {
		cid := cids[i]
		go func(cid *encoding.CID) {
			if err := q.QueueTask(func(ctx context.Context) error {
				ret := pinQueueResult{
					success: true,
					error:   nil,
					cid:     cid,
				}
				err := s.pinEntity(ctx, userId, cid)
				if err != nil {
					s.logger.Error("Error pinning entity", zap.Error(err))
					ret.success = false
					ret.error = err
				}

				rets <- ret
				return nil
			}); err != nil {
				s.logger.Error("Error queueing task", zap.Error(err))
				rets <- pinQueueResult{
					success: false,
					error:   err,
					cid:     cid,
				}
			}
		}(cid)
	}

	go func() {
		received := 0
		for ret := range rets {
			b64, err := ret.cid.ToBase64Url()
			if err != nil {
				s.logger.Error("Error encoding CID to base64", zap.Error(err))
				continue
			}

			results[b64] = pinResult{
				Success: ret.success,
				Error:   ret.error,
			}

			received++

			if received == len(cids) {
				q.Release()
			}
		}
	}()

	q.Wait()

	if middleware.CtxAborted(jc.Request.Context()) {
		return
	}
	jc.Encode(&results)
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

	if decodedCid.Type == types.CIDTypeResolver {
		entry, err := s.getNode().Services().Registry().Get(decodedCid.Hash.FullBytes())
		if err != nil {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyResourceNotFound, err))
			return
		}

		decodedCid, err = encoding.CIDFromRegistry(entry.Data())
		if err != nil {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyInternalError, err))
			return
		}
	}

	found := true

	if err := s.accounts.PinByHash(decodedCid.Hash.HashBytes(), userID); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
			return
		}
		found = false
	}

	if !found {
		if isCidManifest(decodedCid) {
			s.accountPinManifest(jc, userID, decodedCid, true)
			return
		} else {
			err = s.pinEntity(jc.Request.Context(), userID, decodedCid)
			if err != nil {
				s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
				return
			}
		}
	} else {
		cids, err := s.getManifestCids(jc.Request.Context(), decodedCid, false)
		if err != nil {
			s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
			return
		}

		for _, cid := range cids {
			if err := s.accounts.PinByHash(cid.Hash.HashBytes(), userID); err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
					return
				}
				err := s.pinEntity(jc.Request.Context(), userID, cid)
				if err != nil {
					s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err))
					return
				}
			}
		}
	}

	jc.ResponseWriter.WriteHeader(http.StatusNoContent)
}

func (s *S5API) pinEntity(ctx context.Context, userId uint, cid *encoding.CID) error {
	found := true

	if err := s.accounts.PinByHash(cid.Hash.HashBytes(), userId); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		found = false
	}

	if found {
		return nil
	}

	dlUriProvider := s.newStorageLocationProvider(&cid.Hash, true, types.StorageLocationTypeFull, types.StorageLocationTypeFile)

	err := dlUriProvider.Start()

	if err != nil {
		return err
	}

	locations, err := dlUriProvider.All()
	if err != nil {
		return err
	}

	locations = lo.FilterMap(locations, func(location storage2.SignedStorageLocation, index int) (storage2.SignedStorageLocation, bool) {
		r := rq.Get(location.Location().BytesURL())
		httpReq, err := r.ParseRequest()

		if err != nil {
			return nil, false
		}

		res, err := http.DefaultClient.Do(httpReq)

		if err != nil {
			err = dlUriProvider.Downvote(location)
			if err != nil {
				s.logger.Error("Error downvoting location", zap.Error(err))
				return nil, false
			}
			return nil, false
		}
		defer func(Body io.ReadCloser) {
			err := Body.Close()
			if err != nil {
				s.logger.Error("Error closing response body", zap.Error(err))
			}
		}(res.Body)

		// Use io.LimitedReader to limit the download size and attempt to detect if there's more data.
		limitedReader := &io.LimitedReader{R: res.Body, N: int64(s.config.Config().Core.PostUploadLimit + 1)}
		data, err := io.ReadAll(limitedReader)
		if err != nil {
			return nil, false
		}

		if !isCidManifest(cid) {
			if limitedReader.N >= 0 && uint64(len(data)) != cid.Size {
				return nil, false
			}
		} else {
			dataCont, err := io.ReadAll(res.Body)
			if err != nil {
				return nil, false
			}

			data = append(data, dataCont...)

			proof, err := s.storage.HashObject(ctx, bytes.NewReader(data))
			if err != nil {
				return nil, false
			}

			if !bytes.Equal(proof.Hash, cid.Hash.HashBytes()) {
				return nil, false
			}
		}

		return location, true
	})

	if len(locations) == 0 {
		return fmt.Errorf("CID could not be found on the network")
	}

	location := locations[0]

	cid64, err := cid.ToBase64Url()
	if err != nil {
		return nil
	}

	if middleware.CtxAborted(ctx) {
		return ctx.Err()
	}

	jobName := fmt.Sprintf("pin-import-%s", cid64)

	if job := s.cron.GetJobByName(jobName); job == nil {
		job := s.cron.RetryableJob(
			cron.RetryableJobParams{
				Name:     jobName,
				Tags:     nil,
				Function: s.pinImportCronJob,
				Args:     []interface{}{cid64, location.Location().BytesURL(), location.Location().OutboardBytesURL(), userId},
				Attempt:  0,
				Limit:    10,
				After:    nil,
				Error:    nil,
			},
		)

		_, err = s.cron.CreateJob(job)
		if err != nil {
			return nil
		}
	}

	return nil
}

type dirTryFiles []string
type dirErrorPages map[int]string

func (d *dirTryFiles) UnmarshalText(data []byte) error {
	var out []string

	err := json.Unmarshal(data, &out)
	if err != nil {
		return err
	}

	*d = out

	return nil
}

func (d *dirErrorPages) UnmarshalText(data []byte) error {
	var out map[int]string

	err := json.Unmarshal(data, &out)
	if err != nil {
		return err
	}

	*d = out

	return nil
}

func (s *S5API) directoryUpload(jc jape.Context) {

	// Decode form fields
	var (
		tryFiles   dirTryFiles
		errorPages dirErrorPages
		name       string
	)

	if err := jc.DecodeForm("tryFiles", &tryFiles); err != nil || jc.DecodeForm("errorPages", &errorPages) != nil || jc.DecodeForm("name", &name) != nil {
		return
	}

	// Verify content type
	if contentType := jc.Request.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "multipart/form-data") {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, fmt.Errorf("expected multipart/form-data content type, got %s", contentType)))
		return
	}

	// Parse multipart form with size limit from config
	if err := jc.Request.ParseMultipartForm(int64(s.config.Config().Core.PostUploadLimit)); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyInvalidOperation, err))
		return
	}

	uploads, err := s.processMultipartFiles(jc.Request)
	if err != nil {
		s.sendErrorResponse(jc, err)
		return
	}

	var webappErrorPages s5libmetadata.WebAppErrorPages

	for code, page := range errorPages {
		webappErrorPages[code] = page
	}

	// Generate metadata for the directory upload
	app, err := s.createAppMetadata(name, tryFiles, webappErrorPages, uploads)
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
			filename := extractMPFilename(fileHeader.Header)
			if filename == "" {
				return nil, NewS5Error(ErrKeyInvalidOperation, fmt.Errorf("filename not found in multipart file header"))
			}

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

			err = s.accounts.PinByHash(upload.Hash, user)
			if err != nil {
				return nil, NewS5Error(ErrKeyStorageOperationFailed, err)
			}

			uploadMap[filename] = upload
		}
	}

	return uploadMap, nil
}

func (s *S5API) createAppMetadata(name string, tryFiles []string, errorPages s5libmetadata.WebAppErrorPages, uploads map[string]*metadata.UploadMetadata) (*s5libmetadata.WebAppMetadata, error) {
	filesMap := s5libmetadata.NewWebAppFileMap()

	for filename, upload := range uploads {
		hash := upload.Hash

		cid, err := encoding.CIDFromHash(hash, upload.Size, types.CIDTypeRaw, types.HashTypeBlake3)
		if err != nil {
			return nil, NewS5Error(ErrKeyInternalError, err, "Failed to create CID for file: "+filename)
		}
		filesMap.Put(filename, s5libmetadata.WebAppMetadataFileReference{
			Cid:         cid,
			ContentType: upload.MimeType,
		})
	}

	filesMap.Sort()

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

func (s *S5API) uploadAppMetadata(appData *s5libmetadata.WebAppMetadata, r *http.Request) (string, error) {
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

	err = s.accounts.PinByHash(upload.Hash, userId)
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
	dlUriProvider := s.newStorageLocationProvider(&decodedCid.Hash, false, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

	if err := dlUriProvider.Start(); err != nil {
		s.sendErrorResponse(jc, NewS5Error(ErrKeyStorageOperationFailed, err, "Failed to start URI provider"))
		return
	}

	locations, err := node.Services().Storage().GetCachedStorageLocations(&decodedCid.Hash, []types.StorageLocationType{
		types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge,
	}, true)
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

	dlUriProvider := s.newStorageLocationProvider(&cidDecoded.Hash, true, types.StorageLocationTypeFull, types.StorageLocationTypeFile, types.StorageLocationTypeBridge)

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

	dlUriProvider := s.newStorageLocationProvider(decodedHash, false, typeIntList...)

	err = dlUriProvider.Start()
	if jc.Check("error starting search", err) != nil {
		return
	}

	_, err = dlUriProvider.Next()
	if jc.Check("error fetching locations", err) != nil {
		return
	}

	locations, err := s.getNode().Services().Storage().GetCachedStorageLocations(decodedHash, typeIntList, true)
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
		_ = jc.Error(errors.New("Raw CIDs do not have metadata"), http.StatusUnsupportedMediaType)
		return

	case types.CIDTypeResolver:
		_ = jc.Error(errors.New("Resolver CIDs not yet supported"), http.StatusUnsupportedMediaType)
		return
	}

	meta, err := s.getNode().Services().Storage().GetMetadataByCID(cidDecoded)

	if jc.Check("error getting metadata", err) != nil {
		s.logger.Error("error getting metadata", zap.Error(err))
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
	var typ types.CIDType
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
		typ = cidDecoded.Type
	}

	file := s.newFile(FileParams{
		Hash: hashBytes,
		Type: typ,
	})

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

func (s *S5API) newStorageLocationProvider(hash *encoding.Multihash, excludeSelf bool, types ...types.StorageLocationType) storage2.StorageLocationProvider {

	excludeNodes := make([]*encoding.NodeId, 0)

	if excludeSelf {
		excludeNodes = append(excludeNodes, s.getNode().NodeId())
	}

	return provider.NewStorageLocationProvider(provider.StorageLocationProviderParams{
		Services:      s.getNode().Services(),
		Hash:          hash,
		LocationTypes: types,
		ServiceParams: service.ServiceParams{
			Logger: s.logger,
			Config: s.getNode().Config(),
			Db:     s.getNode().Db(),
		},
		ExcludeNodes: excludeNodes,
	})
}

func (s *S5API) newFile(params FileParams) *S5File {
	params.Protocol = s.protocol
	params.Metadata = s.metadata
	params.Storage = s.storage
	params.Tus = s.tusHandler

	return NewFile(params)
}

func (s *S5API) pinImportCronJob(cid string, url string, proofUrl string, userId uint) error {
	ctx := context.Background()

	// Parse CID early to avoid unnecessary operations if it fails.
	parsedCid, err := encoding.CIDFromString(cid)
	if err != nil {
		s.logger.Error("error parsing cid", zap.Error(err))
		return err
	}

	// Function to streamline error handling and closing of response body.
	closeBody := func(body io.ReadCloser) {
		if err := body.Close(); err != nil {
			s.logger.Error("error closing response body", zap.Error(err))
		}
	}

	// Inline fetching and reading body, directly incorporating your checks.
	fetchAndProcess := func(fetchUrl string) ([]byte, error) {
		req, err := rq.Get(fetchUrl).ParseRequest()
		if err != nil {
			s.logger.Error("error parsing request", zap.Error(err))
			return nil, err
		}

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			s.logger.Error("error executing request", zap.Error(err))
			return nil, err
		}
		defer closeBody(res.Body)

		if res.StatusCode != http.StatusOK {
			errMsg := "error fetching URL: " + fetchUrl
			s.logger.Error(errMsg, zap.String("status", res.Status))
			return nil, fmt.Errorf(errMsg+" with status: %s", res.Status)
		}

		data, err := io.ReadAll(res.Body)
		if err != nil {
			s.logger.Error("error reading response body", zap.Error(err))
			return nil, err
		}
		return data, nil
	}

	saveAndPin := func(upload *metadata.UploadMetadata) error {
		upload.UserID = userId
		if err := s.metadata.SaveUpload(ctx, *upload); err != nil {
			return err
		}

		if err := s.accounts.PinByHash(parsedCid.Hash.HashBytes(), userId); err != nil {
			return err
		}

		return nil
	}
	// Fetch file and process if under post upload limit.
	if parsedCid.Size <= s.config.Config().Core.PostUploadLimit {
		fileData, err := fetchAndProcess(url)
		if err != nil {
			return err // Error logged in fetchAndProcess
		}

		hash, err := s.storage.HashObject(ctx, bytes.NewReader(fileData))
		if err != nil {
			s.logger.Error("error hashing object", zap.Error(err))
			return err
		}

		if !bytes.Equal(hash.Hash, parsedCid.Hash.HashBytes()) {
			return fmt.Errorf("hash mismatch")
		}

		upload, err := s.storage.UploadObject(ctx, s5.GetStorageProtocol(s.protocol), bytes.NewReader(fileData), nil, hash)
		if err != nil {
			return err
		}

		err = saveAndPin(upload)
		if err != nil {
			return err
		}

		return nil
	}

	// Fetch proof.
	proof, err := fetchAndProcess(proofUrl)
	if err != nil {
		return err
	}

	baoProof := bao.Result{
		Hash:   parsedCid.Hash.HashBytes(),
		Proof:  proof,
		Length: uint(parsedCid.Size),
	}

	client, err := s.storage.S3Client(ctx)
	if err != nil {
		s.logger.Error("error getting s3 client", zap.Error(err))
		return err
	}

	req, err := rq.Get(url).ParseRequest()
	if err != nil {
		s.logger.Error("error parsing request", zap.Error(err))
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Error("error executing request", zap.Error(err))
		return err
	}
	defer closeBody(res.Body)

	verifier := bao.NewVerifier(res.Body, baoProof, s.logger)
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			s.logger.Error("error closing verifier stream", zap.Error(err))
		}

	}(verifier)

	if parsedCid.Size < storage.S3_MULTIPART_MIN_PART_SIZE {
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(s.config.Config().Core.Storage.S3.BufferBucket),
			Key:           aws.String(cid),
			Body:          verifier,
			ContentLength: aws.Int64(int64(parsedCid.Size)),
		})
		if err != nil {
			s.logger.Error("error uploading object", zap.Error(err))
			return err
		}
	} else {
		err := s.storage.S3MultipartUpload(ctx, verifier, s.config.Config().Core.Storage.S3.BufferBucket, cid, parsedCid.Size)
		if err != nil {
			s.logger.Error("error uploading object", zap.Error(err))
			return err
		}
	}

	upload, err := s.storage.UploadObject(ctx, s5.GetStorageProtocol(s.protocol), nil, &renter.MultiPartUploadParams{
		ReaderFactory: func(start uint, end uint) (io.ReadCloser, error) {
			rangeHeader := "bytes=%d-"
			if end != 0 {
				rangeHeader += "%d"
				rangeHeader = fmt.Sprintf("bytes=%d-%d", start, end)
			} else {
				rangeHeader = fmt.Sprintf("bytes=%d-", start)
			}
			object, err := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(s.config.Config().Core.Storage.S3.BufferBucket),
				Key:    aws.String(cid),
				Range:  aws.String(rangeHeader),
			})

			if err != nil {
				return nil, err
			}

			return object.Body, nil
		},
		Bucket:          s.config.Config().Core.Storage.S3.BufferBucket,
		FileName:        s5.GetStorageProtocol(s.protocol).EncodeFileName(parsedCid.Hash.HashBytes()),
		Size:            parsedCid.Size,
		UploadIDHandler: nil,
	}, &baoProof)

	if err != nil {
		s.logger.Error("error uploading object", zap.Error(err))
		return err
	}

	err = saveAndPin(upload)
	if err != nil {
		return err
	}

	return nil
}

func (s *S5API) Domain() string {
	return router.BuildSubdomain(s, s.config)
}

func (s *S5API) AuthTokenName() string {
	return "s5-auth-token"
}

func isCidManifest(cid *encoding.CID) bool {
	mTypes := []types.CIDType{
		types.CIDTypeMetadataMedia,
		types.CIDTypeMetadataWebapp,
		types.CIDTypeUserIdentity,
		types.CIDTypeDirectory,
	}

	return slices.Contains(mTypes, cid.Type)
}

func extractMPFilename(header textproto.MIMEHeader) string {
	cd := header.Get("Content-Disposition")
	if cd == "" {
		return ""
	}

	_, params, err := mime.ParseMediaType(cd)
	if err != nil {
		return ""
	}

	filename := params["filename"]
	if filename == "" {
		return ""
	}

	return filename
}

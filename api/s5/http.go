package s5

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	emailverifier "github.com/AfterShip/email-verifier"
	"go.sia.tech/jape"
	"go.uber.org/zap"
	"io"
	"mime/multipart"
	"net/http"
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
)

type HttpHandler struct {
	portal   interfaces.Portal
	verifier *emailverifier.Verifier
}

func NewHttpHandler(portal interfaces.Portal) *HttpHandler {

	verifier := emailverifier.NewVerifier()

	verifier.DisableSMTPCheck()
	verifier.DisableGravatarCheck()
	verifier.DisableDomainSuggest()
	verifier.DisableAutoUpdateDisposable()

	return &HttpHandler{
		portal:   portal,
		verifier: verifier,
	}
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

	jc.Encode(&SmallUploadResponse{
		CID: cidStr,
	})
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

	if len(decodedKey) != 33 && int(decodedKey[0]) != int(types.HashTypeEd25519) {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	result := h.portal.Database().Create(&models.S5Challenge{
		Pubkey:    pubkey,
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
		Type:      "register",
	})

	if result.Error != nil {
		_ = jc.Error(errAccountGenerateChallengeErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountGenerateChallenge, zap.Error(err))
		return
	}

	jc.Encode(&AccountRegisterChallengeResponse{
		Challenge: base64.RawURLEncoding.EncodeToString(challenge),
	})
}

func (h *HttpHandler) AccountRegister(jc jape.Context) {
	var request AccountRegisterRequest

	if jc.Decode(&request) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errAccountRegisterErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountRegister, zap.Error(err))
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

	result := h.portal.Database().Model(&models.S5Challenge{}).Where(&models.S5Challenge{Pubkey: request.Pubkey, Type: "register"}).First(&challenge)

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

	verify, _ := h.verifier.Verify(request.Email)

	if !verify.Syntax.Valid {
		errored(errInvalidEmail)
		return
	}

	accountExists, _ := h.portal.Accounts().EmailExists(request.Email)

	if accountExists {
		errored(errEmailAlreadyExists)
		return
	}

	pubkeyExists, _ := h.portal.Accounts().PubkeyExists(request.Pubkey)

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

	newAccount, err := h.portal.Accounts().CreateAccount(request.Email, string(passwd))
	if err != nil {
		errored(errAccountRegisterErr)
		return
	}

	rawPubkey := hex.EncodeToString(decodedKey[1:])

	err = h.portal.Accounts().AddPubkeyToAccount(*newAccount, rawPubkey)
	if err != nil {
		errored(errAccountRegisterErr)
		return
	}

	jwt, err := h.portal.Accounts().LoginPubkey(rawPubkey)
	if err != nil {
		errored(errAccountRegisterErr)
		return
	}

	result = h.portal.Database().Delete(&challenge)

	if result.Error != nil {
		errored(errAccountRegisterErr)
		return
	}

	setAuthCookie(jwt, jc)
}

func (h *HttpHandler) AccountLoginChallenge(jc jape.Context) {
	var pubkey string
	if jc.DecodeForm("pubKey", &pubkey) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errAccountLoginErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountLogin, zap.Error(err))
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
		errored(errAccountGenerateChallengeErr)
		return
	}

	if len(decodedKey) != 33 && int(decodedKey[0]) != int(types.HashTypeEd25519) {
		errored(errPubkeyNotSupported)
		return
	}

	pubkeyExists, _ := h.portal.Accounts().PubkeyExists(hex.EncodeToString(decodedKey[1:]))

	if pubkeyExists {
		errored(errPubkeyNotExist)
		return
	}

	result := h.portal.Database().Create(&models.S5Challenge{
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

func (h *HttpHandler) AccountLogin(jc jape.Context) {
	var request AccountLoginRequest

	if jc.Decode(&request) != nil {
		return
	}

	errored := func(err error) {
		_ = jc.Error(errAccountLoginErr, http.StatusInternalServerError)
		h.portal.Logger().Error(errAccountLogin, zap.Error(err))
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

	result := h.portal.Database().Model(&models.S5Challenge{}).Where(&models.S5Challenge{Pubkey: request.Pubkey, Type: "login"}).First(&challenge)

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

	jwt, err := h.portal.Accounts().LoginPubkey(request.Pubkey)

	if err != nil {
		errored(errAccountLoginErr)
		return
	}

	result = h.portal.Database().Delete(&challenge)

	if result.Error != nil {
		errored(errAccountLoginErr)
		return
	}

	setAuthCookie(jwt, jc)
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

package controller

import (
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/controller/request"
	"git.lumeweb.com/LumeWeb/portal/controller/response"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/joomcode/errorx"
	"github.com/kataras/iris/v12"
	"github.com/kataras/jwt"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"strings"
	"time"
)

var sharedKey = []byte("sercrethatmaycontainch@r$32chars")

var blocklist *jwt.Blocklist

func init() {
	blocklist = jwt.NewBlocklist(1 * time.Hour)
}

type AuthController struct {
	Ctx iris.Context
}

// verifyPassword compares the provided plaintext password with a hashed password and returns an error if they don't match.
func verifyPassword(hashedPassword, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	if err != nil {
		return errors.New("invalid email or password")
	}
	return nil
}

// generateToken generates a JWT token for the given account ID.
func generateToken(maxAge time.Duration) (string, error) {
	// Define the JWT claims.
	claim := jwt.Claims{
		Expiry:   time.Now().Add(time.Hour * 24).Unix(), // Token expires in 24 hours.
		IssuedAt: time.Now().Unix(),
	}

	token, err := jwt.Sign(jwt.HS256, sharedKey, claim, jwt.MaxAge(maxAge))

	if err != nil {
		logger.Get().Error("failed to sign jwt", zap.Error(err))
		return "", err
	}

	return string(token), nil
}

func generateAndSaveLoginToken(accountID uint, maxAge time.Duration) (string, error) {
	// Generate a JWT token for the authenticated user.
	token, err := generateToken(maxAge)
	if err != nil {
		logger.Get().Error("failed to generate token", zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %s", err)
	}

	verifiedToken, _ := jwt.Verify(jwt.HS256, sharedKey, []byte(token), blocklist)
	var claim *jwt.Claims

	_ = verifiedToken.Claims(&claim)

	// Save the token to the database.
	session := model.LoginSession{
		Account:    model.Account{ID: accountID},
		Token:      token,
		Expiration: claim.ExpiresAt(),
	}

	if err := db.Get().Create(&session).Error; err != nil {
		msg := "failed to save token"
		logger.Get().Error(msg, zap.Error(err))
		return "", errorx.Decorate(err, msg)
	}

	return token, nil
}

func generateAndSaveChallengeToken(accountID uint, maxAge time.Duration) (string, error) {
	// Generate a JWT token for the authenticated user.
	token, err := generateToken(maxAge)
	if err != nil {
		logger.Get().Error("failed to generate token", zap.Error(err))
		return "", fmt.Errorf("failed to generate token: %s", err)
	}

	verifiedToken, _ := jwt.Verify(jwt.HS256, sharedKey, []byte(token), blocklist)
	var claim *jwt.Claims

	_ = verifiedToken.Claims(&claim)

	// Save the token to the database.
	keyChallenge := model.KeyChallenge{
		AccountID:  accountID,
		Challenge:  token,
		Expiration: claim.ExpiresAt(),
	}

	if err := db.Get().Create(&keyChallenge).Error; err != nil {
		msg := "failed to save token"
		logger.Get().Error(msg, zap.Error(err))
		return "", errorx.Decorate(err, msg)
	}

	return token, nil
}

// PostLogin handles the POST /api/auth/login request to authenticate a user and return a JWT token.
func (a *AuthController) PostLogin() {
	ri, success := tryParseRequest(request.LoginRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.LoginRequest)

	// Retrieve the account for the given email.
	account := model.Account{}
	if err := db.Get().Where("email = ?", r.Email).First(&account).Error; err != nil {
		msg := "invalid email or password"
		logger.Get().Debug(msg, zap.Error(err))
		a.Ctx.StopWithError(iris.StatusBadRequest, errors.New(msg))
		return
	}

	if account.Password == nil || len(*account.Password) == 0 {
		msg := "only pubkey login is supported"
		logger.Get().Debug(msg)
		a.Ctx.StopWithError(iris.StatusBadRequest, errors.New(msg))
		return
	}

	// Verify the provided password against the hashed password stored in the database.
	if err := verifyPassword(*account.Password, r.Password); err != nil {
		msg := "invalid email or password"
		logger.Get().Debug(msg, zap.Error(err))
		a.Ctx.StopWithError(iris.StatusBadRequest, errors.New(msg))
		return
	}

	// Generate a JWT token for the authenticated user.
	token, err := generateAndSaveLoginToken(account.ID, 24*time.Hour)
	if err != nil {
		logger.Get().Debug("failed to generate token", zap.Error(err))
		a.Ctx.StopWithError(iris.StatusInternalServerError, fmt.Errorf("failed to generate token: %s", err))
		return
	}

	// Return the JWT token to the client.
	err = a.Ctx.JSON(&response.LoginResponse{Token: token})
	if err != nil {
		logger.Get().Error("failed to generate response", zap.Error(err))
	}
}

// PostChallenge handles the POST /api/auth/pubkey/challenge request to generate a challenge for a user's public key.
func (a *AuthController) PostPubkeyChallenge() {
	ri, success := tryParseRequest(request.PubkeyChallengeRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := (ri).(*request.PubkeyChallengeRequest)

	r.Pubkey = strings.ToLower(r.Pubkey)

	// Retrieve the account for the given email.
	account := model.Key{}
	if err := db.Get().Where("pubkey = ?", r.Pubkey).First(&account).Error; err != nil {
		a.Ctx.StopWithError(iris.StatusBadRequest, errors.New("invalid pubkey"))
		return
	}

	// Generate a random challenge string.
	challenge, err := generateAndSaveChallengeToken(account.AccountID, time.Minute)
	if err != nil {
		a.Ctx.StopWithError(iris.StatusInternalServerError, errors.New("failed to generate challenge"))
		return
	}

	// Return the challenge to the client.
	err = a.Ctx.JSON(&response.ChallengeResponse{Challenge: challenge})
	if err != nil {
		logger.Get().Error("failed to create response", zap.Error(err))
	}
}

// PostKeyLogin handles the POST /api/auth/pubkey/login request to authenticate a user using a public key challenge and return a JWT token.
func (a *AuthController) PostPubkeyLogin() {
	ri, success := tryParseRequest(request.PubkeyLoginRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.PubkeyLoginRequest)

	r.Pubkey = strings.ToLower(r.Pubkey)
	r.Signature = strings.ToLower(r.Signature)

	// Retrieve the key challenge for the given challenge.
	challenge := model.KeyChallenge{}
	if err := db.Get().Where("challenge = ?", r.Challenge).First(&challenge).Error; err != nil {
		msg := "invalid key challenge"
		logger.Get().Debug(msg, zap.Error(err), zap.String("challenge", r.Challenge))
		a.Ctx.StopWithError(iris.StatusBadRequest, errorx.RejectedOperation.New(msg))
		return
	}

	verifiedToken, err := jwt.Verify(jwt.HS256, sharedKey, []byte(r.Challenge), blocklist)
	if err != nil {
		msg := fmt.Sprintf("invalid key challenge: %s", err.Error())
		logger.Get().Debug(msg, zap.Error(err), zap.String("challenge", r.Challenge))
		a.Ctx.StopWithError(iris.StatusBadRequest, errorx.RejectedOperation.New(msg))
		return
	}

	rawPubKey, err := hex.DecodeString(r.Pubkey)
	if err != nil {
		msg := fmt.Sprintf("invalid pubkey: %s", err.Error())
		logger.Get().Debug(msg, zap.Error(err), zap.String("pubkey", r.Pubkey))
		a.Ctx.StopWithError(iris.StatusBadRequest, errorx.RejectedOperation.New(msg))
		return
	}

	rawSignature, err := hex.DecodeString(r.Signature)
	if err != nil {
		msg := fmt.Sprintf("invalid signature: %s", err.Error())
		logger.Get().Debug(msg, zap.Error(err), zap.String("signature", r.Signature))
		a.Ctx.StopWithError(iris.StatusBadRequest, errorx.RejectedOperation.New(msg))
		return
	}

	publicKeyDecoded := ed25519.PublicKey(rawPubKey)

	// Verify the challenge signature.
	if !ed25519.Verify(publicKeyDecoded, []byte(r.Challenge), rawSignature) {
		msg := "invalid challenge"
		logger.Get().Debug(msg, zap.Error(err), zap.String("challenge", r.Challenge))
		a.Ctx.StopWithError(iris.StatusBadRequest, errorx.RejectedOperation.New(msg))
	}

	// Generate a JWT token for the authenticated user.
	token, err := generateAndSaveLoginToken(challenge.AccountID, 24*time.Hour)
	if err != nil {
		a.Ctx.StopWithError(iris.StatusInternalServerError, errorx.RejectedOperation.Wrap(err, "failed to generate token"))
		return
	}

	err = blocklist.InvalidateToken(verifiedToken.Token, verifiedToken.StandardClaims)
	if err != nil {
		msg := "failed to invalidate token"
		logger.Get().Error(msg, zap.Error(err), zap.String("token", hex.EncodeToString(verifiedToken.Token)))
		a.Ctx.StopWithError(iris.StatusInternalServerError, errorx.RejectedOperation.Wrap(err, msg))
		return
	}

	if err := db.Get().Delete(&challenge).Error; err != nil {
		msg := "failed to delete key challenge"
		logger.Get().Error(msg, zap.Error(err), zap.Any("key_challenge", challenge))
		a.Ctx.StopWithError(iris.StatusBadRequest, errorx.RejectedOperation.New(msg))
		return
	}

	// Return the JWT token to the client.
	err = a.Ctx.JSON(&response.LoginResponse{Token: token})
	if err != nil {
		logger.Get().Error("failed to create response", zap.Error(err))
	}

}

// PostLogout handles the POST /api/auth/logout request to invalidate a JWT token.
func (a *AuthController) PostLogout() {
	ri, success := tryParseRequest(request.LogoutRequest{}, a.Ctx)
	if !success {
		return
	}

	r, _ := ri.(*request.LogoutRequest)

	// Verify the provided token.
	claims, err := jwt.Verify(jwt.HS256, sharedKey, []byte(r.Token), blocklist)
	if err != nil {
		msg := "invalid token"
		logger.Get().Debug(msg, zap.Error(err))
		a.Ctx.StopWithError(iris.StatusBadRequest, errors.New(msg))
		return
	}

	err = blocklist.InvalidateToken(claims.Token, claims.StandardClaims)
	if err != nil {
		msg := "failed to invalidate token"
		logger.Get().Error(msg, zap.Error(err), zap.String("token", hex.EncodeToString(claims.Token)))
		a.Ctx.StopWithError(iris.StatusBadRequest, errors.New(msg))
		return
	}

	// Return a success response to the client.
	a.Ctx.StatusCode(iris.StatusNoContent)
}

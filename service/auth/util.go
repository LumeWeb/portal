package auth

import (
	"errors"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/kataras/iris/v12"
	"github.com/kataras/jwt"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
	"strconv"
	"strings"
	"time"
)

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

	token, err := jwt.Sign(jwt.EdDSA, jwtKey, claim, jwt.MaxAge(maxAge))

	if err != nil {
		logger.Get().Error(ErrFailedSignJwt.Error(), zap.Error(err))
		return "", err
	}

	return string(token), nil
}

func generateAndSaveLoginToken(accountID uint, maxAge time.Duration) (string, error) {
	// Generate a JWT token for the authenticated user.
	token, err := generateToken(maxAge)
	if err != nil {
		logger.Get().Error(ErrFailedGenerateToken.Error())
		return "", ErrFailedGenerateToken
	}

	verifiedToken, _ := jwt.Verify(jwt.EdDSA, jwtKey, []byte(token), blocklist)
	var claim *jwt.Claims

	_ = verifiedToken.Claims(&claim)

	// Save the token to the database.
	session := model.LoginSession{
		Account:    model.Account{ID: accountID},
		Token:      token,
		Expiration: claim.ExpiresAt(),
	}

	if err := db.Get().Create(&session).Error; err != nil {
		logger.Get().Error(ErrFailedSaveToken.Error(), zap.Uint("account_id", accountID), zap.Duration("max_age", maxAge))
		return "", ErrFailedSaveToken
	}

	return token, nil
}

func generateAndSaveChallengeToken(accountID uint, maxAge time.Duration) (string, error) {
	// Generate a JWT token for the authenticated user.
	token, err := generateToken(maxAge)
	if err != nil {
		logger.Get().Error(ErrFailedGenerateToken.Error(), zap.Error(err))
		return "", ErrFailedGenerateToken
	}

	verifiedToken, _ := jwt.Verify(jwt.EdDSA, jwtKey, []byte(token), blocklist)
	var claim *jwt.Claims

	_ = verifiedToken.Claims(&claim)

	// Save the token to the database.
	keyChallenge := model.KeyChallenge{
		AccountID:  accountID,
		Challenge:  token,
		Expiration: claim.ExpiresAt(),
	}

	if err := db.Get().Create(&keyChallenge).Error; err != nil {
		logger.Get().Error(ErrFailedSaveToken.Error(), zap.Error(err))
		return "", ErrFailedSaveToken
	}

	return token, nil
}

func GetRequestAuthCode(ctx iris.Context) string {
	authHeader := ctx.GetHeader("Authorization")
	if authHeader == "" {
		return ""
	}

	// pure check: authorization header format must be Bearer {token}
	authHeaderParts := strings.Split(authHeader, " ")
	if len(authHeaderParts) != 2 || strings.ToLower(authHeaderParts[0]) != "bearer" {
		return ""
	}

	return authHeaderParts[1]
}

func GetCurrentUserId(ctx iris.Context) uint {
	usr := ctx.User()

	if usr == nil {
		return 0
	}

	sid, _ := usr.GetID()
	userID, _ := strconv.Atoi(sid)

	return uint(userID)
}

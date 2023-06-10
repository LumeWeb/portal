package auth

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"git.lumeweb.com/LumeWeb/portal/config"
	"git.lumeweb.com/LumeWeb/portal/db"
	"git.lumeweb.com/LumeWeb/portal/logger"
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/kataras/jwt"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var jwtKey = ed25519.PrivateKey{}

var blocklist *jwt.Blocklist

var (
	ErrInvalidEmailPassword       = errors.New("Invalid email or password")
	ErrPubkeyOnly                 = errors.New("Only pubkey login is supported")
	ErrFailedGenerateToken        = errors.New("Failed to generate token")
	ErrFailedGenerateKeyChallenge = errors.New("Failed to generate key challenge")
	ErrFailedSignJwt              = errors.New("Failed to sign jwt")
	ErrFailedSaveToken            = errors.New("Failed to sign token")
	ErrFailedDeleteKeyChallenge   = errors.New("Failed to delete key challenge")
	ErrFailedInvalidateToken      = errors.New("Failed to invalidate token")
	ErrInvalidKeyChallenge        = errors.New("Invalid key challenge")
	ErrInvalidPubkey              = errors.New("Invalid pubkey")
	ErrInvalidSignature           = errors.New("Invalid signature")
	ErrInvalidToken               = errors.New("Invalid token")
)

func Init() {
	blocklist = jwt.NewBlocklist(0)

	configFile := viper.ConfigFileUsed()

	var jwtPemPath string
	jwtPemName := "jwt.pem"

	if configFile == "" {
		jwtPemPath = path.Join(config.ConfigFilePaths[0], jwtPemName)
	} else {
		jwtPemPath = path.Join(filepath.Dir(configFile), jwtPemName)
	}

	if _, err := os.Stat(jwtPemPath); err != nil {
		_, private, err := ed25519.GenerateKey(nil)
		if err != nil {
			logger.Get().Fatal("Failed to compute JWT private key", zap.Error(err))
		}

		privateBytes, err := x509.MarshalPKCS8PrivateKey(private)

		if err != nil {
			logger.Get().Fatal("Failed to create marshal private key", zap.Error(err))
		}

		var pemPrivateBlock = &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: privateBytes,
		}

		pemPrivateFile, err := os.Create(jwtPemPath)

		if err != nil {
			logger.Get().Fatal("Failed to create empty file for JWT private PEM", zap.Error(err))
		}

		err = pem.Encode(pemPrivateFile, pemPrivateBlock)
		if err != nil {
			logger.Get().Fatal("Failed to write JWT private PEM", zap.Error(err))
		}

		jwtKey = private
	} else {
		data, err := os.ReadFile(jwtPemPath)
		if err != nil {
			logger.Get().Fatal("Failed to read JWT private PEM", zap.Error(err))
		}

		pemBlock, _ := pem.Decode(data)
		if err != nil {
			logger.Get().Fatal("Failed to decode JWT private PEM", zap.Error(err))
		}

		privateBytes, err := x509.ParsePKCS8PrivateKey(pemBlock.Bytes)

		if err != nil {
			logger.Get().Fatal("Failed to unmarshal JWT private PEM", zap.Error(err))
		}

		jwtKey = privateBytes.(ed25519.PrivateKey)
	}

}

func LoginWithPassword(email string, password string) (string, error) {
	// Retrieve the account for the given email.
	account := model.Account{}
	if err := db.Get().Model(&account).Where("email = ?", email).First(&account).Error; err != nil {
		logger.Get().Debug(ErrInvalidEmailPassword.Error(), zap.String("email", email))
		return "", ErrInvalidEmailPassword
	}

	if account.Password == nil || len(*account.Password) == 0 {
		logger.Get().Debug(ErrPubkeyOnly.Error(), zap.String("email", email))
		return "", ErrPubkeyOnly
	}

	// Verify the provided password against the hashed password stored in the database.
	if err := verifyPassword(*account.Password, password); err != nil {
		logger.Get().Debug(ErrPubkeyOnly.Error(), zap.String("email", email))
		return "", ErrInvalidEmailPassword
	}

	// Generate a JWT token for the authenticated user.
	token, err := generateAndSaveLoginToken(account.ID, 24*time.Hour)
	if err != nil {
		return "", err
	}

	return token, nil
}

func LoginWithPubkey(pubkey string, challenge string, signature string) (string, error) {
	pubkey = strings.ToLower(pubkey)
	signature = strings.ToLower(signature)

	// Retrieve the key challenge for the given challenge.
	challengeObj := model.KeyChallenge{}
	if err := db.Get().Model(challengeObj).Where("challenge = ?", challenge).First(&challengeObj).Error; err != nil {
		logger.Get().Debug(ErrInvalidKeyChallenge.Error(), zap.Error(err), zap.String("challenge", challenge))
		return "", ErrInvalidKeyChallenge
	}

	verifiedToken, err := jwt.Verify(jwt.EdDSA, jwtKey, []byte(challenge), blocklist)
	if err != nil {
		logger.Get().Debug(ErrInvalidKeyChallenge.Error(), zap.Error(err), zap.String("challenge", challenge))
		return "", ErrInvalidKeyChallenge
	}

	rawPubKey, err := hex.DecodeString(pubkey)
	if err != nil {
		logger.Get().Debug(ErrInvalidPubkey.Error(), zap.Error(err), zap.String("pubkey", pubkey))
		return "", ErrInvalidPubkey
	}

	rawSignature, err := hex.DecodeString(signature)
	if err != nil {
		logger.Get().Debug(ErrInvalidPubkey.Error(), zap.Error(err), zap.String("signature", pubkey))
		return "", ErrInvalidSignature
	}

	publicKeyDecoded := ed25519.PublicKey(rawPubKey)

	// Verify the challenge signature.
	if !ed25519.Verify(publicKeyDecoded, []byte(challenge), rawSignature) {
		logger.Get().Debug(ErrInvalidKeyChallenge.Error(), zap.Error(err), zap.String("challenge", challenge))
		return "", ErrInvalidKeyChallenge
	}

	// Generate a JWT token for the authenticated user.
	token, err := generateAndSaveLoginToken(challengeObj.AccountID, 24*time.Hour)
	if err != nil {
		return "", err
	}

	err = blocklist.InvalidateToken(verifiedToken.Token, verifiedToken.StandardClaims)
	if err != nil {
		logger.Get().Error(ErrFailedInvalidateToken.Error(), zap.Error(err), zap.String("pubkey", pubkey), zap.ByteString("token", verifiedToken.Token), zap.String("challenge", challenge))
		return "", ErrFailedInvalidateToken
	}

	if err := db.Get().Delete(&challengeObj).Error; err != nil {
		logger.Get().Debug(ErrFailedDeleteKeyChallenge.Error(), zap.Error(err))
		return "", ErrFailedDeleteKeyChallenge
	}

	return token, nil
}

func GeneratePubkeyChallenge(pubkey string) (string, error) {
	pubkey = strings.ToLower(pubkey)

	// Retrieve the account for the given email.
	account := model.Key{}
	if err := db.Get().Where("pubkey = ?", pubkey).First(&account).Error; err != nil {
		logger.Get().Debug("failed to query pubkey", zap.Error(err))
		return "", errors.New("invalid pubkey")
	}

	// Generate a random challenge string.
	challenge, err := generateAndSaveChallengeToken(account.AccountID, time.Minute)
	if err != nil {
		logger.Get().Error(ErrFailedGenerateKeyChallenge.Error())
		return "", ErrFailedGenerateKeyChallenge
	}

	return challenge, nil
}

func Logout(token string) error {
	// Verify the provided token.
	claims, err := jwt.Verify(jwt.EdDSA, jwtKey, []byte(token), blocklist)
	if err != nil {
		logger.Get().Debug(ErrInvalidToken.Error(), zap.Error(err))
		return ErrInvalidToken
	}

	err = blocklist.InvalidateToken(claims.Token, claims.StandardClaims)
	if err != nil {
		logger.Get().Error(ErrFailedInvalidateToken.Error(), zap.Error(err), zap.String("token", token))
		return ErrFailedInvalidateToken
	}

	// Retrieve the key challenge for the given challenge.
	session := model.LoginSession{}
	if err := db.Get().Model(session).Where("token = ?", token).First(&session).Error; err != nil {
		logger.Get().Debug(ErrFailedInvalidateToken.Error(), zap.Error(err), zap.String("token", token))
		return ErrFailedInvalidateToken
	}

	db.Get().Delete(&session)

	return nil
}

func VerifyLoginToken(token string) (*model.Account, error) {
	uvt, err := jwt.Decode([]byte(token))
	if err != nil {
		return nil, ErrInvalidToken
	}

	var claim jwt.Claims

	err = uvt.Claims(&claim)
	if err != nil {
		return nil, ErrInvalidToken
	}

	session := model.LoginSession{}
	if err := db.Get().Model(session).Where("token = ?", token).First(&session).Error; err != nil {
		logger.Get().Debug(ErrInvalidToken.Error(), zap.Error(err), zap.String("token", token))
		return nil, ErrInvalidToken
	}

	_, err = jwt.Verify(jwt.EdDSA, jwtKey, []byte(token), blocklist)
	if err != nil {
		db.Get().Delete(&session)
		return nil, err
	}

	return &session.Account, nil
}

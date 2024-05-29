package core

import (
	"fmt"
	"net/http"
)

const (
	// Account creation errors
	ErrKeyAccountCreationFailed = "ErrAccountCreationFailed"
	ErrKeyEmailAlreadyExists    = "ErrEmailAlreadyExists"
	ErrKeyUpdatingSameEmail     = "ErrUpdatingSameEmail"
	ErrKeyPasswordHashingFailed = "ErrPasswordHashingFailed"

	// Account lookup and existence verification errors
	ErrKeyUserNotFound      = "ErrUserNotFound"
	ErrKeyPublicKeyNotFound = "ErrPublicKeyNotFound"

	// Authentication and login errors
	ErrKeyInvalidLogin          = "ErrInvalidLogin"
	ErrKeyInvalidPassword       = "ErrInvalidPassword"
	ErrKeyInvalidOTPCode        = "ErrInvalidOTPCode"
	ErrKeyOTPVerificationFailed = "ErrOTPVerificationFailed"
	ErrKeyLoginFailed           = "ErrLoginFailed"
	ErrKeyHashingFailed         = "ErrHashingFailed"

	// Account update errors
	ErrKeyAccountUpdateFailed    = "ErrAccountUpdateFailed"
	ErrKeyAccountAlreadyVerified = "ErrAccountAlreadyVerified"

	// JWT generation errors
	ErrKeyJWTGenerationFailed = "ErrJWTGenerationFailed"

	// OTP management errors
	ErrKeyOTPGenerationFailed = "ErrOTPGenerationFailed"
	ErrKeyOTPEnableFailed     = "ErrOTPEnableFailed"
	ErrKeyOTPDisableFailed    = "ErrOTPDisableFailed"

	// Public key management errors
	ErrKeyAddPublicKeyFailed = "ErrAddPublicKeyFailed"
	ErrKeyPublicKeyExists    = "ErrPublicKeyExists"

	// Pin management errors
	ErrKeyPinAddFailed        = "ErrPinAddFailed"
	ErrKeyPinDeleteFailed     = "ErrPinDeleteFailed"
	ErrKeyPinsRetrievalFailed = "ErrPinsRetrievalFailed"

	// General errors
	ErrKeyDatabaseOperationFailed = "ErrDatabaseOperationFailed"

	// Security token errors
	ErrKeySecurityTokenExpired = "ErrSecurityTokenExpired"
	ErrKeySecurityInvalidToken = "ErrSecurityInvalidToken"
)

var defaultErrorMessages = map[string]string{
	// Account creation errors
	ErrKeyAccountCreationFailed: "Account creation failed due to an internal error.",
	ErrKeyEmailAlreadyExists:    "The email address provided is already in use.",
	ErrKeyPasswordHashingFailed: "Failed to secure the password, please try again later.",
	ErrKeyUpdatingSameEmail:     "The email address provided is the same as your current one.",

	// Account lookup and existence verification errors
	ErrKeyUserNotFound:      "The requested user was not found.",
	ErrKeyPublicKeyNotFound: "The specified public key was not found.",
	ErrKeyHashingFailed:     "Failed to hash the password.",

	// Authentication and login errors
	ErrKeyInvalidLogin:          "The login credentials provided are invalid.",
	ErrKeyInvalidPassword:       "The password provided is incorrect.",
	ErrKeyInvalidOTPCode:        "The OTP code provided is invalid or expired.",
	ErrKeyOTPVerificationFailed: "OTP verification failed, please try again.",
	ErrKeyLoginFailed:           "Login failed due to an internal error.",

	// Account update errors
	ErrKeyAccountUpdateFailed:    "Failed to update account information.",
	ErrKeyAccountAlreadyVerified: "Account is already verified.",

	// JWT generation errors
	ErrKeyJWTGenerationFailed: "Failed to generate a new JWT token.",

	// OTP management errors
	ErrKeyOTPGenerationFailed: "Failed to generate a new OTP secret.",
	ErrKeyOTPEnableFailed:     "Enabling OTP authentication failed.",
	ErrKeyOTPDisableFailed:    "Disabling OTP authentication failed.",

	// Public key management errors
	ErrKeyAddPublicKeyFailed: "Adding the public key to the account failed.",
	ErrKeyPublicKeyExists:    "The public key already exists for this account.",

	// Pin management errors
	ErrKeyPinAddFailed:        "Failed to add the pin.",
	ErrKeyPinDeleteFailed:     "Failed to delete the pin.",
	ErrKeyPinsRetrievalFailed: "Failed to retrieve pins.",

	// General errors
	ErrKeyDatabaseOperationFailed: "A database operation failed.",

	// Security token errors
	ErrKeySecurityTokenExpired: "The security token has expired.",
	ErrKeySecurityInvalidToken: "The security token is invalid.",
}

var (
	ErrorCodeToHttpStatus = map[string]int{
		// Account creation errors
		ErrKeyAccountCreationFailed: http.StatusInternalServerError,
		ErrKeyEmailAlreadyExists:    http.StatusConflict,
		ErrKeyPasswordHashingFailed: http.StatusInternalServerError,

		// Account lookup and existence verification errors
		ErrKeyUserNotFound:      http.StatusNotFound,
		ErrKeyPublicKeyNotFound: http.StatusNotFound,

		// Authentication and login errors
		ErrKeyInvalidLogin:          http.StatusUnauthorized,
		ErrKeyInvalidPassword:       http.StatusUnauthorized,
		ErrKeyInvalidOTPCode:        http.StatusBadRequest,
		ErrKeyOTPVerificationFailed: http.StatusBadRequest,
		ErrKeyLoginFailed:           http.StatusInternalServerError,

		// Account update errors
		ErrKeyAccountUpdateFailed:    http.StatusInternalServerError,
		ErrKeyAccountAlreadyVerified: http.StatusConflict,

		// JWT generation errors
		ErrKeyJWTGenerationFailed: http.StatusInternalServerError,

		// OTP management errors
		ErrKeyOTPGenerationFailed: http.StatusInternalServerError,
		ErrKeyOTPEnableFailed:     http.StatusInternalServerError,
		ErrKeyOTPDisableFailed:    http.StatusInternalServerError,

		// Public key management errors
		ErrKeyAddPublicKeyFailed: http.StatusInternalServerError,
		ErrKeyPublicKeyExists:    http.StatusConflict,

		// Pin management errors
		ErrKeyPinAddFailed:        http.StatusInternalServerError,
		ErrKeyPinDeleteFailed:     http.StatusInternalServerError,
		ErrKeyPinsRetrievalFailed: http.StatusInternalServerError,

		// General errors
		ErrKeyDatabaseOperationFailed: http.StatusInternalServerError,
		ErrKeyHashingFailed:           http.StatusInternalServerError,

		// Security token errors
		ErrKeySecurityTokenExpired: http.StatusUnauthorized,
		ErrKeySecurityInvalidToken: http.StatusUnauthorized,
	}
)

type AccountError struct {
	Key     string // A unique identifier for the error type
	Message string // Human-readable error message
	Err     error  // Underlying error, if any
}

func (e *AccountError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func NewAccountError(key string, err error, customMessage ...string) *AccountError {
	message, exists := defaultErrorMessages[key]
	if !exists {
		message = "An unknown error occurred"
	}
	if len(customMessage) > 0 {
		message = customMessage[0]
	}
	return &AccountError{
		Key:     key,
		Message: message,
		Err:     err,
	}
}

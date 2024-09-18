package core

import (
	"fmt"
	"net/http"
)

type AccountErrorType string

const (
	// Account creation errors
	ErrKeyAccountCreationFailed AccountErrorType = "ErrAccountCreationFailed"
	ErrKeyEmailAlreadyExists    AccountErrorType = "ErrEmailAlreadyExists"
	ErrKeyUpdatingSameEmail     AccountErrorType = "ErrUpdatingSameEmail"
	ErrKeyPasswordHashingFailed AccountErrorType = "ErrPasswordHashingFailed"

	// Account lookup and existence verification errors
	ErrKeyUserNotFound      AccountErrorType = "ErrUserNotFound"
	ErrKeyPublicKeyNotFound AccountErrorType = "ErrPublicKeyNotFound"

	// Account deletion errors
	ErrKeyAccountDeletionRequestAlreadyExists AccountErrorType = "ErrAccountDeletionRequestAlreadyExists"

	// Authentication and login errors
	ErrKeyInvalidLogin           AccountErrorType = "ErrInvalidLogin"
	ErrKeyInvalidPassword        AccountErrorType = "ErrInvalidPassword"
	ErrKeyInvalidOTPCode         AccountErrorType = "ErrInvalidOTPCode"
	ErrKeyOTPVerificationFailed  AccountErrorType = "ErrOTPVerificationFailed"
	ErrKeyLoginFailed            AccountErrorType = "ErrLoginFailed"
	ErrKeyHashingFailed          AccountErrorType = "ErrHashingFailed"
	ErrKeyAccountPendingDeletion AccountErrorType = "ErrAccountPendingDeletion"

	// Account update errors
	ErrKeyAccountUpdateFailed    AccountErrorType = "ErrAccountUpdateFailed"
	ErrKeyAccountAlreadyVerified AccountErrorType = "ErrAccountAlreadyVerified"

	// JWT generation errors
	ErrKeyJWTGenerationFailed AccountErrorType = "ErrJWTGenerationFailed"

	// OTP management errors
	ErrKeyOTPGenerationFailed AccountErrorType = "ErrOTPGenerationFailed"
	ErrKeyOTPEnableFailed     AccountErrorType = "ErrOTPEnableFailed"
	ErrKeyOTPDisableFailed    AccountErrorType = "ErrOTPDisableFailed"

	// Public key management errors
	ErrKeyAddPublicKeyFailed AccountErrorType = "ErrAddPublicKeyFailed"
	ErrKeyPublicKeyExists    AccountErrorType = "ErrPublicKeyExists"

	// Pin management errors
	ErrKeyPinAddFailed        AccountErrorType = "ErrPinAddFailed"
	ErrKeyPinDeleteFailed     AccountErrorType = "ErrPinDeleteFailed"
	ErrKeyPinsRetrievalFailed AccountErrorType = "ErrPinsRetrievalFailed"

	// General errors
	ErrKeyDatabaseOperationFailed = "ErrDatabaseOperationFailed"

	// Security token errors
	ErrKeySecurityTokenExpired AccountErrorType = "ErrSecurityTokenExpired"
	ErrKeySecurityInvalidToken AccountErrorType = "ErrSecurityInvalidToken"

	// Internal errors
	ErrKeyAccountSubdomainNotSet AccountErrorType = "ErrAccountSubdomainNotSet"
)

var defaultErrorMessages = map[AccountErrorType]string{
	// Account creation errors
	ErrKeyAccountCreationFailed: "Account creation failed due to an internal error.",
	ErrKeyEmailAlreadyExists:    "The email address provided is already in use.",
	ErrKeyPasswordHashingFailed: "Failed to secure the password, please try again later.",
	ErrKeyUpdatingSameEmail:     "The email address provided is the same as your current one.",

	// Account lookup and existence verification errors
	ErrKeyUserNotFound:      "The requested user was not found.",
	ErrKeyPublicKeyNotFound: "The specified public key was not found.",
	ErrKeyHashingFailed:     "Failed to hash the password.",

	// Account deletion errors
	ErrKeyAccountDeletionRequestAlreadyExists: "An account deletion request already exists for this account.",

	// Authentication and login errors
	ErrKeyInvalidLogin:           "The login credentials provided are invalid.",
	ErrKeyInvalidPassword:        "The password provided is incorrect.",
	ErrKeyInvalidOTPCode:         "The OTP code provided is invalid or expired.",
	ErrKeyOTPVerificationFailed:  "OTP verification failed, please try again.",
	ErrKeyLoginFailed:            "Login failed due to an internal error.",
	ErrKeyAccountPendingDeletion: "This account is pending deletion.",

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

	// Internal errors
	ErrKeyAccountSubdomainNotSet: "The account subdomain is not set.",
}

var (
	ErrorCodeToHttpStatus = map[AccountErrorType]int{
		// Account creation errors
		ErrKeyAccountCreationFailed: http.StatusInternalServerError,
		ErrKeyEmailAlreadyExists:    http.StatusConflict,
		ErrKeyPasswordHashingFailed: http.StatusInternalServerError,

		// Account lookup and existence verification errors
		ErrKeyUserNotFound:      http.StatusNotFound,
		ErrKeyPublicKeyNotFound: http.StatusNotFound,

		// Account deletion errors
		ErrKeyAccountDeletionRequestAlreadyExists: http.StatusConflict,

		// Authentication and login errors
		ErrKeyInvalidLogin:           http.StatusUnauthorized,
		ErrKeyInvalidPassword:        http.StatusUnauthorized,
		ErrKeyInvalidOTPCode:         http.StatusBadRequest,
		ErrKeyOTPVerificationFailed:  http.StatusBadRequest,
		ErrKeyLoginFailed:            http.StatusInternalServerError,
		ErrKeyAccountPendingDeletion: http.StatusForbidden,

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

		// Internal errors
		ErrKeyAccountSubdomainNotSet: http.StatusInternalServerError,
	}
)

type AccountError struct {
	Key     AccountErrorType // A unique identifier for the error type
	Message string           // Human-readable error message
	Err     error            // Underlying error, if any
}

func (e *AccountError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AccountError) IsErrorType(key AccountErrorType) bool {
	return e.Key == key
}

func (e *AccountError) HttpStatus() int {
	if status, exists := ErrorCodeToHttpStatus[e.Key]; exists {
		return status
	}
	return http.StatusInternalServerError
}

func NewAccountError(key AccountErrorType, err error, customMessage ...string) *AccountError {
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

func IsAccountError(err error) bool {
	if err == nil {
		return false
	}
	if _, ok := err.(*AccountError); ok {
		return true
	}

	return false
}

func AsAccountError(err error) *AccountError {
	if err == nil {
		return nil
	}
	if e, ok := err.(*AccountError); ok {
		return e
	}
	return nil
}

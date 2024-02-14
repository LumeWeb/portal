package account

import "fmt"

const (
	// Account creation errors
	ErrKeyAccountCreationFailed = "ErrAccountCreationFailed"
	ErrKeyEmailAlreadyExists    = "ErrEmailAlreadyExists"
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
	ErrKeyAccountUpdateFailed = "ErrAccountUpdateFailed"

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
)

var defaultErrorMessages = map[string]string{
	// Account creation errors
	ErrKeyAccountCreationFailed: "Account creation failed due to an internal error.",
	ErrKeyEmailAlreadyExists:    "The email address provided is already in use.",
	ErrKeyPasswordHashingFailed: "Failed to secure the password, please try again later.",

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
	ErrKeyAccountUpdateFailed: "Failed to update account information.",

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
}

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

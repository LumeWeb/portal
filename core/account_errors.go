package core

import (
	"net/http"
)

const accountErrorNamespace = "account"

type AccountErrorType = ErrorType

const (
	// Account creation errors
	ErrKeyAccountCreationFailed ErrorType = "ErrAccountCreationFailed"
	ErrKeyEmailAlreadyExists    ErrorType = "ErrEmailAlreadyExists"
	ErrKeyUpdatingSameEmail     ErrorType = "ErrUpdatingSameEmail"
	ErrKeyPasswordHashingFailed ErrorType = "ErrPasswordHashingFailed"

	// Account role errors
	ErrKeyAssigningAdminRoleFailed ErrorType = "ErrAssigningAdminRoleFailed"
	ErrKeyAssigningUserRoleFailed  ErrorType = "ErrAssigningUserRoleFailed"

	// Account lookup and existence verification errors
	ErrKeyUserNotFound      ErrorType = "ErrUserNotFound"
	ErrKeyKeyIdentityNotFound ErrorType = "ErrKeyIdentityNotFound"

	// Account deletion errors
	ErrKeyAccountDeletionRequestAlreadyExists ErrorType = "ErrAccountDeletionRequestAlreadyExists"

	// Authentication and login errors
	ErrKeyInvalidLogin           ErrorType = "ErrInvalidLogin"
	ErrKeyInvalidPassword        ErrorType = "ErrInvalidPassword"
	ErrKeyInvalidOTPCode         ErrorType = "ErrInvalidOTPCode"
	ErrKeyOTPVerificationFailed  ErrorType = "ErrOTPVerificationFailed"
	ErrKeyLoginFailed            ErrorType = "ErrLoginFailed"
	ErrKeyHashingFailed          ErrorType = "ErrHashingFailed"
	ErrKeyAccountPendingDeletion ErrorType = "ErrAccountPendingDeletion"
	ErrKeyAccountNotVerified     ErrorType = "ErrAccountNotVerified"

	// Account update errors
	ErrKeyAccountUpdateFailed    ErrorType = "ErrAccountUpdateFailed"
	ErrKeyAccountAlreadyVerified ErrorType = "ErrAccountAlreadyVerified"

	// JWT generation errors
	ErrKeyJWTGenerationFailed ErrorType = "ErrJWTGenerationFailed"

	// OTP management errors
	ErrKeyOTPGenerationFailed ErrorType = "ErrOTPGenerationFailed"
	ErrKeyOTPEnableFailed     ErrorType = "ErrOTPEnableFailed"
	ErrKeyOTPDisableFailed    ErrorType = "ErrOTPDisableFailed"

	// Key identity management errors
	ErrKeyAddKeyIdentityFailed ErrorType = "ErrAddKeyIdentityFailed"
	ErrKeyKeyIdentityExists    ErrorType = "ErrKeyIdentityExists"

	// Pin management errors
	ErrKeyPinAddFailed        ErrorType = "ErrPinAddFailed"
	ErrKeyPinDeleteFailed     ErrorType = "ErrPinDeleteFailed"
	ErrKeyPinsRetrievalFailed ErrorType = "ErrPinsRetrievalFailed"

	// General errors
	ErrKeyDatabaseOperationFailed ErrorType = "ErrDatabaseOperationFailed"

	// Security token errors
	ErrKeySecurityTokenExpired ErrorType = "ErrSecurityTokenExpired"
	ErrKeySecurityInvalidToken ErrorType = "ErrSecurityInvalidToken"

	// Internal errors
	ErrKeyAccountSubdomainNotSet ErrorType = "ErrAccountSubdomainNotSet"
)

var defaultAccountErrorMessages = map[ErrorType]ErrorDefinition{
	// Account creation errors
	ErrKeyAccountCreationFailed: {
		Key:     ErrKeyAccountCreationFailed,
		Message: "Account creation failed due to an internal error.",
	},
	ErrKeyEmailAlreadyExists: {
		Key:     ErrKeyEmailAlreadyExists,
		Message: "The email address provided is already in use.",
	},
	ErrKeyPasswordHashingFailed: {
		Key:     ErrKeyPasswordHashingFailed,
		Message: "Failed to secure the password, please try again later.",
	},
	ErrKeyUpdatingSameEmail: {
		Key:     ErrKeyUpdatingSameEmail,
		Message: "The email address provided is the same as your current one.",
	},

	// Account role errors
	ErrKeyAssigningAdminRoleFailed: {
		Key:     ErrKeyAssigningAdminRoleFailed,
		Message: "Failed to assign the admin role to the account.",
	},
	ErrKeyAssigningUserRoleFailed: {
		Key:     ErrKeyAssigningUserRoleFailed,
		Message: "Failed to assign the user role to the account.",
	},

	// Account lookup and existence verification errors
	ErrKeyUserNotFound: {
		Key:     ErrKeyUserNotFound,
		Message: "The requested user was not found.",
	},
	ErrKeyKeyIdentityNotFound: {
		Key:     ErrKeyKeyIdentityNotFound,
		Message: "The specified key identity was not found.",
	},
	ErrKeyHashingFailed: {
		Key:     ErrKeyHashingFailed,
		Message: "Failed to hash the password.",
	},

	// Account deletion errors
	ErrKeyAccountDeletionRequestAlreadyExists: {
		Key:     ErrKeyAccountDeletionRequestAlreadyExists,
		Message: "An account deletion request already exists for this account.",
	},

	// Authentication and login errors
	ErrKeyInvalidLogin: {
		Key:     ErrKeyInvalidLogin,
		Message: "The login credentials provided are invalid.",
	},
	ErrKeyInvalidPassword: {
		Key:     ErrKeyInvalidPassword,
		Message: "The password provided is incorrect.",
	},
	ErrKeyInvalidOTPCode: {
		Key:     ErrKeyInvalidOTPCode,
		Message: "The OTP code provided is invalid or expired.",
	},
	ErrKeyOTPVerificationFailed: {
		Key:     ErrKeyOTPVerificationFailed,
		Message: "OTP verification failed, please try again.",
	},
	ErrKeyLoginFailed: {
		Key:     ErrKeyLoginFailed,
		Message: "Login failed due to an internal error.",
	},
	ErrKeyAccountPendingDeletion: {
		Key:     ErrKeyAccountPendingDeletion,
		Message: "This account is pending deletion.",
	},
	ErrKeyAccountNotVerified: {
		Key:     ErrKeyAccountNotVerified,
		Message: "The account is not verified.",
	},

	// Account update errors
	ErrKeyAccountUpdateFailed: {
		Key:     ErrKeyAccountUpdateFailed,
		Message: "Failed to update account information.",
	},
	ErrKeyAccountAlreadyVerified: {
		Key:     ErrKeyAccountAlreadyVerified,
		Message: "Account is already verified.",
	},

	// JWT generation errors
	ErrKeyJWTGenerationFailed: {
		Key:     ErrKeyJWTGenerationFailed,
		Message: "Failed to generate a new JWT token.",
	},

	// OTP management errors
	ErrKeyOTPGenerationFailed: {
		Key:     ErrKeyOTPGenerationFailed,
		Message: "Failed to generate a new OTP secret.",
	},
	ErrKeyOTPEnableFailed: {
		Key:     ErrKeyOTPEnableFailed,
		Message: "Enabling OTP authentication failed.",
	},
	ErrKeyOTPDisableFailed: {
		Key:     ErrKeyOTPDisableFailed,
		Message: "Disabling OTP authentication failed.",
	},

	// Key identity management errors
	ErrKeyAddKeyIdentityFailed: {
		Key:     ErrKeyAddKeyIdentityFailed,
		Message: "Adding the key identity to the account failed.",
	},
	ErrKeyKeyIdentityExists: {
		Key:     ErrKeyKeyIdentityExists,
		Message: "The key identity already exists for this account.",
	},

	// Pin management errors
	ErrKeyPinAddFailed: {
		Key:     ErrKeyPinAddFailed,
		Message: "Failed to add the pin.",
	},
	ErrKeyPinDeleteFailed: {
		Key:     ErrKeyPinDeleteFailed,
		Message: "Failed to delete the pin.",
	},
	ErrKeyPinsRetrievalFailed: {
		Key:     ErrKeyPinsRetrievalFailed,
		Message: "Failed to retrieve pins.",
	},

	// General errors
	ErrKeyDatabaseOperationFailed: {
		Key:     ErrKeyDatabaseOperationFailed,
		Message: "A database operation failed.",
	},

	// Security token errors
	ErrKeySecurityTokenExpired: {
		Key:     ErrKeySecurityTokenExpired,
		Message: "The security token has expired.",
	},
	ErrKeySecurityInvalidToken: {
		Key:     ErrKeySecurityInvalidToken,
		Message: "The security token is invalid.",
	},

	// Internal errors
	ErrKeyAccountSubdomainNotSet: {
		Key:     ErrKeyAccountSubdomainNotSet,
		Message: "The account subdomain is not set.",
	},
}

var (
	accountErrorCodeToHttpStatus = map[ErrorType]int{
		// Account creation errors
		ErrKeyAccountCreationFailed: http.StatusInternalServerError,
		ErrKeyEmailAlreadyExists:    http.StatusConflict,
		ErrKeyPasswordHashingFailed: http.StatusInternalServerError,

		// Account role errors
		ErrKeyAssigningAdminRoleFailed: http.StatusInternalServerError,
		ErrKeyAssigningUserRoleFailed:  http.StatusInternalServerError,

		// Account lookup and existence verification errors
		ErrKeyUserNotFound:      http.StatusNotFound,
		ErrKeyKeyIdentityNotFound: http.StatusNotFound,

		// Account deletion errors
		ErrKeyAccountDeletionRequestAlreadyExists: http.StatusConflict,

		// Authentication and login errors
		ErrKeyInvalidLogin:           http.StatusUnauthorized,
		ErrKeyInvalidPassword:        http.StatusUnauthorized,
		ErrKeyInvalidOTPCode:         http.StatusBadRequest,
		ErrKeyOTPVerificationFailed:  http.StatusBadRequest,
		ErrKeyLoginFailed:            http.StatusInternalServerError,
		ErrKeyAccountPendingDeletion: http.StatusForbidden,
		ErrKeyAccountNotVerified:     http.StatusForbidden,

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
		ErrKeyAddKeyIdentityFailed: http.StatusInternalServerError,
		ErrKeyKeyIdentityExists:    http.StatusConflict,

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

func init() {
	MustRegisterNamespace(accountErrorNamespace)
	MustRegisterDefaultErrorMessages(accountErrorNamespace, defaultAccountErrorMessages)
	MustRegisterErrorCodes(accountErrorNamespace, accountErrorCodeToHttpStatus)
}

// NewAccountError creates a new Error instance using the core error registry.
func NewAccountError(key ErrorType, err error, args ...interface{}) *Error {
	return NewError(accountErrorNamespace, key, err, args...)
}

// IsAccountError checks if the error is an account error.
func IsAccountError(err error) bool {
	return IsNamespaceError(err, accountErrorNamespace)
}

// AsAccountError casts the error to a Error if possible.
func AsAccountError(err error) *Error {
	if err == nil {
		return nil
	}

	e, ok := err.(*Error)
	if !ok {
		return nil
	}

	if !e.IsNamespace(accountErrorNamespace) {
		return nil
	}

	return e
}

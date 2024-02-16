package s5

import (
	"fmt"
	"net/http"
)

// S5-specific error keys
const (
	// File-related errors
	ErrKeyFileUploadFailed     = "ErrFileUploadFailed"
	ErrKeyFileDownloadFailed   = "ErrFileDownloadFailed"
	ErrKeyMetadataFetchFailed  = "ErrMetadataFetchFailed"
	ErrKeyInvalidFileFormat    = "ErrInvalidFileFormat"
	ErrKeyUnsupportedFileType  = "ErrUnsupportedFileType"
	ErrKeyFileProcessingFailed = "ErrFileProcessingFailed"

	// Storage and data handling errors
	ErrKeyStorageOperationFailed = "ErrStorageOperationFailed"
	ErrKeyResourceNotFound       = "ErrResourceNotFound"
	ErrKeyResourceLimitExceeded  = "ErrResourceLimitExceeded"
	ErrKeyDataIntegrityError     = "ErrDataIntegrityError"

	// User and permission errors
	ErrKeyPermissionDenied     = "ErrPermissionDenied"
	ErrKeyInvalidOperation     = "ErrInvalidOperation"
	ErrKeyAuthenticationFailed = "ErrAuthenticationFailed"
	ErrKeyAuthorizationFailed  = "ErrAuthorizationFailed"

	// Network and communication errors
	ErrKeyNetworkError       = "ErrNetworkError"
	ErrKeyServiceUnavailable = "ErrServiceUnavailable"

	// General errors
	ErrKeyInternalError      = "ErrInternalError"
	ErrKeyConfigurationError = "ErrConfigurationError"
	ErrKeyOperationTimeout   = "ErrOperationTimeout"
)

// Default error messages for S5-specific errors
var defaultErrorMessages = map[string]string{
	ErrKeyFileUploadFailed:       "File upload failed due to an internal error.",
	ErrKeyFileDownloadFailed:     "File download failed.",
	ErrKeyMetadataFetchFailed:    "Failed to fetch metadata for the resource.",
	ErrKeyInvalidFileFormat:      "Invalid file format provided.",
	ErrKeyUnsupportedFileType:    "Unsupported file type.",
	ErrKeyFileProcessingFailed:   "Failed to process the file.",
	ErrKeyStorageOperationFailed: "Storage operation failed unexpectedly.",
	ErrKeyResourceNotFound:       "The specified resource was not found.",
	ErrKeyResourceLimitExceeded:  "The operation exceeded the resource limit.",
	ErrKeyDataIntegrityError:     "Data integrity check failed.",
	ErrKeyPermissionDenied:       "Permission denied for the requested operation.",
	ErrKeyInvalidOperation:       "Invalid or unsupported operation requested.",
	ErrKeyAuthenticationFailed:   "Authentication failed.",
	ErrKeyAuthorizationFailed:    "Authorization failed or insufficient permissions.",
	ErrKeyNetworkError:           "Network error or connectivity issue.",
	ErrKeyServiceUnavailable:     "The requested service is temporarily unavailable.",
	ErrKeyInternalError:          "An internal server error occurred.",
	ErrKeyConfigurationError:     "Configuration error or misconfiguration detected.",
	ErrKeyOperationTimeout:       "The operation timed out.",
}

// Mapping of S5-specific error keys to HTTP status codes
var errorCodeToHttpStatus = map[string]int{
	ErrKeyFileUploadFailed:       http.StatusInternalServerError,
	ErrKeyFileDownloadFailed:     http.StatusInternalServerError,
	ErrKeyMetadataFetchFailed:    http.StatusInternalServerError,
	ErrKeyInvalidFileFormat:      http.StatusBadRequest,
	ErrKeyUnsupportedFileType:    http.StatusBadRequest,
	ErrKeyFileProcessingFailed:   http.StatusInternalServerError,
	ErrKeyStorageOperationFailed: http.StatusInternalServerError,
	ErrKeyResourceNotFound:       http.StatusNotFound,
	ErrKeyResourceLimitExceeded:  http.StatusForbidden,
	ErrKeyDataIntegrityError:     http.StatusInternalServerError,
	ErrKeyPermissionDenied:       http.StatusForbidden,
	ErrKeyInvalidOperation:       http.StatusBadRequest,
	ErrKeyAuthenticationFailed:   http.StatusUnauthorized,
	ErrKeyAuthorizationFailed:    http.StatusUnauthorized,
	ErrKeyNetworkError:           http.StatusBadGateway,
	ErrKeyServiceUnavailable:     http.StatusServiceUnavailable,
	ErrKeyInternalError:          http.StatusInternalServerError,
	ErrKeyConfigurationError:     http.StatusInternalServerError,
	ErrKeyOperationTimeout:       http.StatusRequestTimeout,
}

// S5Error struct for representing S5-specific errors
type S5Error struct {
	Key     string
	Message string
	Err     error
}

// Error method to implement the error interface
func (e *S5Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *S5Error) HttpStatus() int {
	if code, exists := errorCodeToHttpStatus[e.Key]; exists {
		return code
	}
	return http.StatusInternalServerError
}

func NewS5Error(key string, err error, customMessage ...string) *S5Error {
	message, exists := defaultErrorMessages[key]
	if !exists {
		message = "An unknown error occurred"
	}
	if len(customMessage) > 0 {
		message = customMessage[0]
	}

	return &S5Error{
		Key:     key,
		Message: message,
		Err:     err,
	}
}

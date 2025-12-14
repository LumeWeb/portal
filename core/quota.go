package core

import "errors"

var (
	// ErrStorageQuotaExceeded is returned when user exceeds storage quota
	ErrStorageQuotaExceeded = errors.New("storage quota exceeded")

	// ErrUploadQuotaExceeded is returned when user exceeds upload quota
	ErrUploadQuotaExceeded = errors.New("upload quota exceeded")

	// ErrDownloadQuotaExceeded is returned when user exceeds download quota
	ErrDownloadQuotaExceeded = errors.New("download quota exceeded")
)

// IsQuotaExceededError checks if the error is any quota exceeded error
func IsQuotaExceededError(err error) bool {
	return errors.Is(err, ErrStorageQuotaExceeded) ||
		errors.Is(err, ErrUploadQuotaExceeded) ||
		errors.Is(err, ErrDownloadQuotaExceeded)
}

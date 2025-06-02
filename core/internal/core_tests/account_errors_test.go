package core_tests

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core"
)

func TestNewAccountError(t *testing.T) {
	t.Run("WithDefaultMessage", func(t *testing.T) {
		errKey := core.ErrKeyEmailAlreadyExists
		underlyingErr := errors.New("database constraint violation")
		accErr := core.NewAccountError(errKey, underlyingErr)

		assert.NotNil(t, accErr)
		assert.Equal(t, errKey, accErr.Key)
		assert.Equal(t, core.AccountErrorType("ErrEmailAlreadyExists"), accErr.Key) // Explicit type check
		assert.Equal(t, "The email address provided is already in use.", accErr.Message)
		assert.Equal(t, underlyingErr, accErr.Err)
		assert.Equal(t, "The email address provided is already in use.: database constraint violation", accErr.Error())
	})

	t.Run("WithCustomMessage", func(t *testing.T) {
		errKey := core.ErrKeyUserNotFound
		underlyingErr := errors.New("user ID 123 not found in DB")
		customMsg := "Could not find the user you were looking for."
		accErr := core.NewAccountError(errKey, underlyingErr, customMsg)

		assert.NotNil(t, accErr)
		assert.Equal(t, errKey, accErr.Key)
		assert.Equal(t, customMsg, accErr.Message)
		assert.Equal(t, underlyingErr, accErr.Err)
		assert.Equal(t, "Could not find the user you were looking for.: user ID 123 not found in DB", accErr.Error())
	})

	t.Run("WithUnknownKey", func(t *testing.T) {
		errKey := core.AccountErrorType("ErrUnknownTestKey")
		underlyingErr := errors.New("some internal issue")
		accErr := core.NewAccountError(errKey, underlyingErr)

		assert.NotNil(t, accErr)
		assert.Equal(t, errKey, accErr.Key)
		assert.Equal(t, "An unknown error occurred", accErr.Message) // Fallback message
		assert.Equal(t, underlyingErr, accErr.Err)
		assert.Equal(t, "An unknown error occurred: some internal issue", accErr.Error())
	})

	t.Run("WithoutUnderlyingError", func(t *testing.T) {
		errKey := core.ErrKeyInvalidPassword
		accErr := core.NewAccountError(errKey, nil)

		assert.NotNil(t, accErr)
		assert.Equal(t, errKey, accErr.Key)
		assert.Equal(t, "The password provided is incorrect.", accErr.Message)
		assert.Nil(t, accErr.Err)
		assert.Equal(t, "The password provided is incorrect.", accErr.Error()) // No underlying error message appended
	})
}

func TestAccountError_Error(t *testing.T) {
	t.Run("WithUnderlyingError", func(t *testing.T) {
		underlyingErr := errors.New("db connection failed")
		accErr := core.NewAccountError(core.ErrKeyDatabaseOperationFailed, underlyingErr)
		expected := "A database operation failed.: db connection failed"
		assert.Equal(t, expected, accErr.Error())
	})

	t.Run("WithoutUnderlyingError", func(t *testing.T) {
		accErr := core.NewAccountError(core.ErrKeyUserNotFound, nil)
		expected := "The requested user was not found."
		assert.Equal(t, expected, accErr.Error())
	})
}

func TestAccountError_IsErrorType(t *testing.T) {
	accErr := core.NewAccountError(core.ErrKeyEmailAlreadyExists, nil)

	assert.True(t, accErr.IsErrorType(core.ErrKeyEmailAlreadyExists))
	assert.False(t, accErr.IsErrorType(core.ErrKeyUserNotFound))
	assert.False(t, accErr.IsErrorType(core.AccountErrorType("SomeOtherKey")))
}

func TestAccountError_HttpStatus(t *testing.T) {
	tests := []struct {
		key          core.AccountErrorType
		expectedCode int
	}{
		{core.ErrKeyEmailAlreadyExists, http.StatusConflict},
		{core.ErrKeyUserNotFound, http.StatusNotFound},
		{core.ErrKeyInvalidLogin, http.StatusUnauthorized},
		{core.ErrKeyInvalidOTPCode, http.StatusBadRequest},
		{core.ErrKeyAccountPendingDeletion, http.StatusForbidden},
		{core.ErrKeyAccountUpdateFailed, http.StatusInternalServerError},
		{core.ErrKeySecurityTokenExpired, http.StatusUnauthorized},
		{core.AccountErrorType("NonExistentKey"), http.StatusInternalServerError}, // Default fallback
	}

	for _, tt := range tests {
		t.Run(string(tt.key), func(t *testing.T) {
			accErr := core.NewAccountError(tt.key, nil)
			assert.Equal(t, tt.expectedCode, accErr.HttpStatus())
		})
	}
}

func TestIsAccountError(t *testing.T) {
	t.Run("IsAccountError", func(t *testing.T) {
		accErr := core.NewAccountError(core.ErrKeyUserNotFound, nil)
		assert.True(t, core.IsAccountError(accErr))
	})

	t.Run("IsNotAccountError_StandardError", func(t *testing.T) {
		stdErr := errors.New("a standard error")
		assert.False(t, core.IsAccountError(stdErr))
	})

	t.Run("IsNotAccountError_Nil", func(t *testing.T) {
		assert.False(t, core.IsAccountError(nil))
	})
}

func TestAsAccountError(t *testing.T) {
	t.Run("IsAccountError", func(t *testing.T) {
		accErr := core.NewAccountError(core.ErrKeyUserNotFound, nil)
		result := core.AsAccountError(accErr)
		assert.NotNil(t, result)
		assert.Equal(t, accErr, result)
	})

	t.Run("IsNotAccountError_StandardError", func(t *testing.T) {
		stdErr := errors.New("a standard error")
		result := core.AsAccountError(stdErr)
		assert.Nil(t, result)
	})

	t.Run("IsNotAccountError_Nil", func(t *testing.T) {
		result := core.AsAccountError(nil)
		assert.Nil(t, result)
	})
}

package core_tests

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

func TestRegisterUploadDataHandler(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	mockHandler := mocks.NewMockUploadDataHandler(t)
	handlerID := "test-handler"

	// Test successful registration
	core.RegisterUploadDataHandler(handlerID, mockHandler)

	// Verify handler was registered
	retrievedHandler, exists := core.GetUploadDataHandler(handlerID)
	assert.True(t, exists)
	assert.Equal(t, mockHandler, retrievedHandler)

	// Test duplicate registration
	assert.PanicsWithValue(t, "upload data handler already registered: "+handlerID, func() {
		core.RegisterUploadDataHandler(handlerID, mockHandler)
	})
}

func TestGetUploadDataHandler(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	mockHandler := mocks.NewMockUploadDataHandler(t)
	handlerID := "test-handler"

	// Test handler not found
	_, exists := core.GetUploadDataHandler("non-existent-handler")
	assert.False(t, exists)

	// Register and test handler found
	core.RegisterUploadDataHandler(handlerID, mockHandler)
	retrievedHandler, exists := core.GetUploadDataHandler(handlerID)
	assert.True(t, exists)
	assert.Equal(t, mockHandler, retrievedHandler)
}

func TestResetUploadHandlers(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	// Register some test upload handlers
	mockHandler1 := mocks.NewMockUploadDataHandler(t)
	mockHandler2 := mocks.NewMockUploadDataHandler(t)
	core.RegisterUploadDataHandler("test-handler-1", mockHandler1)
	core.RegisterUploadDataHandler("test-handler-2", mockHandler2)

	// Check handlers exist
	_, exists := core.GetUploadDataHandler("test-handler-1")
	assert.True(t, exists)
	_, exists = core.GetUploadDataHandler("test-handler-2")
	assert.True(t, exists)

	// Reset upload handlers
	core.ResetUploadHandlers()

	// Check handlers no longer exist
	_, exists = core.GetUploadDataHandler("test-handler-1")
	assert.False(t, exists)
	_, exists = core.GetUploadDataHandler("test-handler-2")
	assert.False(t, exists)
}

func TestUploadDataHandlerMethods(t *testing.T) {
	core.ResetState()
	defer core.ResetState()

	mockHandler := mocks.NewMockUploadDataHandler(t)
	handlerID := "test-handler"
	core.RegisterUploadDataHandler(handlerID, mockHandler)

	t.Run("CreateUploadData", func(t *testing.T) {
		mockHandler.On("CreateUploadData", mock.Anything, mock.Anything, uint(1), mock.Anything).Return(nil).Once()
		err := mockHandler.CreateUploadData(context.Background(), &gorm.DB{}, 1, "test-data")
		assert.NoError(t, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("GetUploadData", func(t *testing.T) {
		expectedData := "test-data"
		mockHandler.On("GetUploadData", mock.Anything, mock.Anything, uint(1)).Return(expectedData, nil).Once()
		data, err := mockHandler.GetUploadData(context.Background(), &gorm.DB{}, 1)
		assert.NoError(t, err)
		assert.Equal(t, expectedData, data)
		mockHandler.AssertExpectations(t)
	})

	t.Run("UpdateUploadData", func(t *testing.T) {
		mockHandler.On("UpdateUploadData", mock.Anything, mock.Anything, uint(1), mock.Anything).Return(nil).Once()
		err := mockHandler.UpdateUploadData(context.Background(), &gorm.DB{}, 1, "updated-data")
		assert.NoError(t, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("DeleteUploadData", func(t *testing.T) {
		mockHandler.On("DeleteUploadData", mock.Anything, mock.Anything, uint(1)).Return(nil).Once()
		err := mockHandler.DeleteUploadData(context.Background(), &gorm.DB{}, 1)
		assert.NoError(t, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("QueryUploadData", func(t *testing.T) {
		mockDB := &gorm.DB{}
		mockHandler.On("QueryUploadData", mock.Anything, mock.Anything, mock.Anything).Return(mockDB).Once()
		result := mockHandler.QueryUploadData(context.Background(), &gorm.DB{}, "query")
		assert.Equal(t, mockDB, result)
		mockHandler.AssertExpectations(t)
	})

	t.Run("CompleteUploadData", func(t *testing.T) {
		mockHandler.On("CompleteUploadData", mock.Anything, mock.Anything, uint(1)).Return(nil).Once()
		err := mockHandler.CompleteUploadData(context.Background(), &gorm.DB{}, 1)
		assert.NoError(t, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("GetUploadDataModel", func(t *testing.T) {
		expectedModel := &models.Upload{}
		mockHandler.On("GetUploadDataModel").Return(expectedModel).Once()
		model := mockHandler.GetUploadDataModel()
		assert.Equal(t, expectedModel, model)
		mockHandler.AssertExpectations(t)
	})
}

func TestUploadServiceInterface(t *testing.T) {
	mockService := mocks.NewMockUploadService(t)

	t.Run("SaveUpload", func(t *testing.T) {
		upload := &models.Upload{}
		mockService.On("SaveUpload", mock.Anything, upload).Return(nil).Once()
		err := mockService.SaveUpload(context.Background(), upload)
		assert.NoError(t, err)
		mockService.AssertExpectations(t)
	})

	t.Run("GetUpload", func(t *testing.T) {
		hash := mocks.NewMockStorageHash(t)
		hash.On("String").Return("mock-hash")
		expectedUpload := &models.Upload{}
		mockService.On("GetUpload", mock.Anything, hash).Return(expectedUpload, nil).Once()
		upload, err := mockService.GetUpload(context.Background(), hash)
		assert.NoError(t, err)
		assert.Equal(t, expectedUpload, upload)
		mockService.AssertExpectations(t)
		hash.AssertExpectations(t)
	})

	t.Run("DeleteUpload", func(t *testing.T) {
		hash := mocks.NewMockStorageHash(t)
		hash.On("String").Return("mock-hash")
		mockService.On("DeleteUpload", mock.Anything, hash).Return(nil).Once()
		err := mockService.DeleteUpload(context.Background(), hash)
		assert.NoError(t, err)
		mockService.AssertExpectations(t)
		hash.AssertExpectations(t)
	})

	t.Run("GetAllUploads", func(t *testing.T) {
		expectedUploads := []*models.Upload{{}, {}}
		mockService.On("GetAllUploads", mock.Anything).Return(expectedUploads, nil).Once()
		uploads, err := mockService.GetAllUploads(context.Background())
		assert.NoError(t, err)
		assert.Len(t, uploads, 2)
		mockService.AssertExpectations(t)
	})

	t.Run("GetUploadByID", func(t *testing.T) {
		expectedUpload := &models.Upload{}
		mockService.On("GetUploadByID", mock.Anything, uint(1)).Return(expectedUpload, nil).Once()
		upload, err := mockService.GetUploadByID(context.Background(), 1)
		assert.NoError(t, err)
		assert.Equal(t, expectedUpload, upload)
		mockService.AssertExpectations(t)
	})

	t.Run("ServiceInterface", func(t *testing.T) {
		mockService.On("ID").Return("test-service").Once()
		id := mockService.ID()
		assert.Equal(t, "test-service", id)
		mockService.AssertExpectations(t)
	})
}

func TestUploadErrors(t *testing.T) {
	mockHandler := mocks.NewMockUploadDataHandler(t)
	handlerID := "test-handler"
	core.RegisterUploadDataHandler(handlerID, mockHandler)

	t.Run("CreateUploadDataError", func(t *testing.T) {
		expectedErr := errors.New("create error")
		mockHandler.On("CreateUploadData", mock.Anything, mock.Anything, uint(1), mock.Anything).Return(expectedErr).Once()
		err := mockHandler.CreateUploadData(context.Background(), &gorm.DB{}, 1, "test-data")
		assert.Equal(t, expectedErr, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("GetUploadDataError", func(t *testing.T) {
		expectedErr := errors.New("get error")
		mockHandler.On("GetUploadData", mock.Anything, mock.Anything, uint(1)).Return(nil, expectedErr).Once()
		_, err := mockHandler.GetUploadData(context.Background(), &gorm.DB{}, 1)
		assert.Equal(t, expectedErr, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("UpdateUploadDataError", func(t *testing.T) {
		expectedErr := errors.New("update error")
		mockHandler.On("UpdateUploadData", mock.Anything, mock.Anything, uint(1), mock.Anything).Return(expectedErr).Once()
		err := mockHandler.UpdateUploadData(context.Background(), &gorm.DB{}, 1, "updated-data")
		assert.Equal(t, expectedErr, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("DeleteUploadDataError", func(t *testing.T) {
		expectedErr := errors.New("delete error")
		mockHandler.On("DeleteUploadData", mock.Anything, mock.Anything, uint(1)).Return(expectedErr).Once()
		err := mockHandler.DeleteUploadData(context.Background(), &gorm.DB{}, 1)
		assert.Equal(t, expectedErr, err)
		mockHandler.AssertExpectations(t)
	})

	t.Run("CompleteUploadDataError", func(t *testing.T) {
		expectedErr := errors.New("complete error")
		mockHandler.On("CompleteUploadData", mock.Anything, mock.Anything, uint(1)).Return(expectedErr).Once()
		err := mockHandler.CompleteUploadData(context.Background(), &gorm.DB{}, 1)
		assert.Equal(t, expectedErr, err)
		mockHandler.AssertExpectations(t)
	})
}

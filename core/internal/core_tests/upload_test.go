package core_tests

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
)

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

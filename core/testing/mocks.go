package testing

import (
	"context"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
)

// MockService is a generic mock implementation of core.Service
type MockService struct {
	IDValue string
}

func (s *MockService) ID() string {
	return s.IDValue
}

// NewMockService creates a new mock service with the given ID
func NewMockService(id string) *MockService {
	return &MockService{IDValue: id}
}

// MockStorageService implements core.StorageService for testing
type MockStorageService struct {
	*MockService
	UploadObjectFunc func(ctx context.Context, request core.StorageUploadRequest) (*models.Upload, error)
	// Add other methods as needed
}

func NewMockStorageService() *MockStorageService {
	return &MockStorageService{
		MockService: NewMockService(core.STORAGE_SERVICE),
	}
}

// MockAuthService implements core.AuthService for testing
type MockAuthService struct {
	*MockService
	LoginPasswordFunc func(email string, password string, ip string, rememberMe bool) (string, *models.User, error)
	// Add other methods as needed
}

func NewMockAuthService() *MockAuthService {
	return &MockAuthService{
		MockService: NewMockService(core.AUTH_SERVICE),
	}
}

// Add more mock implementations as needed

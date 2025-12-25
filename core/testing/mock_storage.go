package testing

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"gorm.io/gorm"
)

var _ core.StorageService = (*MockStorageService)(nil)

// MockStorageService provides a simplified mock for the StorageService that uses a real S3 client.
// It embeds the mock storage service to handle all the storage operations, but overrides S3Client
// to provide a real S3 client configured like the actual storage service.
type MockStorageService struct {
	*mocks.MockStorageService
	ctx               core.Context
	s3Client          *s3.Client
	s3ClientMu        sync.RWMutex
	componentConfig   config.Manager
	componentLogger   *core.Logger
	componentDB       *gorm.DB
}

// NewMockStorageService creates a new MockStorageService with a core context.
func NewMockStorageService(t TB, ctx core.Context) *MockStorageService {
	m := &MockStorageService{
		MockStorageService: mocks.NewMockStorageService(t),
		ctx:                ctx,
	}

	// Auto-wire common expectations
	m.MockStorageService.EXPECT().GetTemporaryUploadDir(mock.Anything).Return("/tmp").Maybe()

	return m
}

// ID returns the service ID.
func (m *MockStorageService) ID() string {
	return core.STORAGE_SERVICE
}

// S3Client returns a real S3 client configured like the storage service.
// This method is overridden to provide a real S3 client instead of a mock.
func (m *MockStorageService) S3Client(ctx context.Context) (*s3.Client, error) {
	m.s3ClientMu.RLock()
	if m.s3Client != nil {
		m.s3ClientMu.RUnlock()
		return m.s3Client, nil
	}
	m.s3ClientMu.RUnlock()

	// Initialize S3 client lazily
	m.s3ClientMu.Lock()
	defer m.s3ClientMu.Unlock()

	// Double-check after acquiring write lock
	if m.s3Client != nil {
		return m.s3Client, nil
	}

	// Use the stored core context to get config
	if m.ctx == nil {
		return nil, fmt.Errorf("mock storage service not initialized with context - use NewMockStorageService")
	}

	// Create S3 client using the same logic as the real storage service
	s3Config := m.ctx.Config().Config().Core.Storage.S3

	awsCfg, err := awsConfig.LoadDefaultConfig(ctx,
		awsConfig.WithRegion(s3Config.Region),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s3Config.AccessKey,
			s3Config.SecretKey,
			"",
		)),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with endpoint configuration
	m.s3Client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if s3Config.Endpoint != "" {
			o.BaseEndpoint = aws.String(ensureHttpPrefix(s3Config.Endpoint))
		}
		o.UsePathStyle = true
	})

	return m.s3Client, nil
}

// ensureHttpPrefix ensures the endpoint has an http:// or https:// prefix
func ensureHttpPrefix(endpoint string) string {
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return "http://" + endpoint
	}
	return endpoint
}

// Config implements core.Component
func (m *MockStorageService) Config() config.Manager {
	return m.componentConfig
}

// SetConfig implements core.Component
func (m *MockStorageService) SetConfig(cfg config.Manager) {
	m.componentConfig = cfg
}

// Context implements core.Component
func (m *MockStorageService) Context() core.Context {
	return m.ctx
}

// SetContext implements core.Component
func (m *MockStorageService) SetContext(ctx core.Context) {
	m.ctx = ctx
}

// Logger implements core.Component
func (m *MockStorageService) Logger() *core.Logger {
	return m.componentLogger
}

// SetLogger implements core.Component
func (m *MockStorageService) SetLogger(logger *core.Logger) {
	m.componentLogger = logger
}

// DB implements core.Component
func (m *MockStorageService) DB() *gorm.DB {
	return m.componentDB
}

// SetDB implements core.Component
func (m *MockStorageService) SetDB(db *gorm.DB) {
	m.componentDB = db
}

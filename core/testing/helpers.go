// Package helpers provides testing helper functions.
package testing

import (
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"testing/fstest"
	"time"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3afero"
	"github.com/spf13/afero"
	"go.lumeweb.com/portal/build"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"go.uber.org/zap"
)

// GetFreeListener finds and returns a listener on a TCP port
func GetFreeListener() (net.Listener, error) {
	addr, err := net.ResolveTCPAddr("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return nil, err
	}

	return l, nil
}

// SetupTestEnvironment creates and initializes a TestContext with common services and configurations.
// It handles resetting state, creating the context, applying options, and booting the environment.
func SetupTestEnvironment(tb TB) (TestContext, error) {
	tb.Helper()

	// Reset test case state (global state remains)
	ResetAllState()

	// Create base test context
	ctx, err := SetupTest(tb)
	if err != nil {
		tb.Fatalf("SetupTest failed: %v", err)
	}

	// Process all options (defaults + global + test case)
	options, err := GetCombinedTestContextOptions()
	if err != nil {
		return ctx, fmt.Errorf("failed to get combined test context options: %w", err)
	}
	ctx, err = ProcessCtxOptions(ctx, options...)
	if err != nil {
		return ctx, fmt.Errorf("failed to process test context options: %w", err)
	}

	// Boot the environment
	err = BootEnvironment(tb, ctx)
	if err != nil {
		return ctx, fmt.Errorf("failed to boot test environment: %w", err)
	}

	return ctx, nil
}

// WithMockS3 configures a test context to use a gofakes3 instance.
// It launches a gofakes3 server, configures the S3 config, and registers cleanup.
// bucketName: The name of the S3 bucket to create.
// configureS3Config: Optional function to further configure the S3Config.
func WithMockS3() TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Create a gofakes3 instance
		tempDir, err := os.MkdirTemp("", "portal-s3-")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp dir: %w", err)
		}

		backend, err := s3afero.SingleBucket("fakebucket", afero.NewBasePathFs(afero.NewOsFs(), tempDir), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create s3 backend: %w", err)
		}
		faker := gofakes3.New(backend)

		// Launch the gofakes3 server
		httpHandler := faker.Server()
		server := &http.Server{Handler: httpHandler}

		// Find an available port
		listener, err := GetFreeListener()
		if err != nil {
			return nil, fmt.Errorf("failed to get free port: %w", err)
		}

		endpoint := fmt.Sprintf("http://127.0.0.1:%d", listener.Addr().(*net.TCPAddr).Port)

		// Channel to signal server is ready
		ready := make(chan struct{})

		go func() {
			close(ready) // Signal server is starting
			if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
				ctx.Logger().Error("gofakes3 server failed", zap.Error(err))
			}
		}()

		// Wait for server to be ready
		select {
		case <-ready:
			// Brief delay to ensure server is listening
			time.Sleep(10 * time.Millisecond)
		case <-time.After(100 * time.Millisecond):
			return nil, fmt.Errorf("timeout waiting for S3 server to start")
		}

		ctx.RegisterCleanup(func() {
			// Shutdown server
			if err = server.Shutdown(ctx.GetContext()); err != nil {
				ctx.Logger().Error("failed to shutdown gofakes3 server",
					zap.Error(err))
			}

			// Remove temp directory
			if err = os.RemoveAll(tempDir); err != nil {
				ctx.Logger().Error("failed to remove temp directory",
					zap.String("path", tempDir),
					zap.Error(err))
			}
		})

		// Configure the S3 config
		s3Config := &config.S3Config{
			BufferBucket: "fakebucket",
			Endpoint:     endpoint,
			Region:       "us-east-1",
			AccessKey:    "FAKEACCESSKEY",
			SecretKey:    "FAKESECRETKEY",
		}

		// Set the S3 config in the test context
		mockConfig := GetRealConfig(ctx)
		configValues := map[string]interface{}{
			"core.storage.s3.buffer_bucket": s3Config.BufferBucket,
			"core.storage.s3.endpoint":      s3Config.Endpoint,
			"core.storage.s3.region":        s3Config.Region,
			"core.storage.s3.access_key":    s3Config.AccessKey,
			"core.storage.s3.secret_key":    s3Config.SecretKey,
		}

		for key, value := range configValues {
			if err := mockConfig.Set(ctx, key, value); err != nil {
				return nil, fmt.Errorf("failed to set s3 config: %w", err)
			}
		}

		return ctx, nil
	}
}

// WithSQLitePluginMigrations registers a mock plugin with the given ID and SQLite migrations.
func WithSQLitePluginMigrations(pluginID string, migrationsFS fs.FS) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		pluginInfo := core.PluginInfo{
			ID:      pluginID,
			Version: build.New("", "", "", "", "", "", ""),
			Migrations: core.DBMigration{
				core.DB_TYPE_SQLITE: migrationsFS,
			},
			WebBundles: core.NewWebBundles(core.NewWebBundle(fstest.MapFS{})),
		}

		core.RegisterPlugin(pluginInfo)
		return ctx, nil
	}
}

// WithKeyIdentityHandler registers a key identity handler for a given type
// directly in the test context. This is useful for tests that need a handler
// without registering a full plugin.
func WithKeyIdentityHandler(keyType string, handler core.KeyIdentityHandler) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		core.RegisterKeyIdentity(keyType, handler)
		return ctx, nil
	}
}

// ShutdownTestContext is a helper for tests that calls all registered exit functions
// and cleans up resources without exiting the process.
func ShutdownTestContext(ctx TestContext) {
	// Perform all cleanup through Teardown which handles cancellation and exit functions
	ctx.Teardown()
}

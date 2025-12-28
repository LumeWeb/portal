// Package testing provides utilities for testing core components
package testing

import (
	"fmt"
	"reflect"

	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/service"
	"go.uber.org/zap"
)

// Private interface types for mock configuration

// mockWithOn represents a mock that has the On method for setting up expectations
type mockWithOn interface {
	On(methodName string, arguments ...any) *mock.Call
}

// WithMockService adds a mock service to the test context by calling the mock constructor
// during the test context's BootEnvironment phase.
func WithMockService(id string, mockConstructor func(tb TB, ctx TestContext) any) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Register a startup function that will create and register the mock later
		startupOpt := core.ContextWithStartupFunc(func(coreCtx core.Context) error {
			tctx, ok := coreCtx.(TestContext)
			if !ok {
				return fmt.Errorf("context is not a TestContext, cannot use WithMockService")
			}

			// Create and register the mock instance
			mockInstance := mockConstructor(tctx.T(), tctx)
			if err := registerServiceInstance(tctx, id, mockInstance); err != nil {
				return fmt.Errorf("failed to register mock service: %w", err)
			}

			return nil
		})

		return ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
	}
}

type MockServiceFactory[T any] func(interface {
	mock.TestingT
	Cleanup(func())
}) *T

// WithMockServiceFactory creates a TestContextBuilderOption that registers a service
// by calling a factory function immediately during the ProcessCtxOptions phase.
// This allows mocks that require the testing.TB instance to be created with the
// correct TB for each individual test run.
// Optional service config can be provided to set up GetConfig() expectations on the mock.
func WithMockServiceFactory[T any](id string, factory MockServiceFactory[T], serviceConfig ...any) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {
		// Create the mock instance immediately using the test's TB
		serviceInstance := factory(ctx.T())

		// Ensure the created instance is not nil
		if serviceInstance == nil {
			return ctx, fmt.Errorf("mock service factory for '%s' returned nil", id)
		}

		switch len(serviceConfig) {
		case 0:
			// no-op
		case 1:
			onMock, ok := any(serviceInstance).(mockWithOn)
			if !ok {
				return ctx, fmt.Errorf("mock service '%s' does not support On(); cannot auto-configure GetConfig()", id)
			}
			// Consider whether GetConfig() should be required or optional when provided.
			// If optional, keep Maybe(); if required, drop it (or set Once()).
			onMock.On("GetConfig").Return(serviceConfig[0], nil).Maybe()
		default:
			return ctx, fmt.Errorf("WithMockServiceFactory('%s') expected at most 1 serviceConfig value, got %d", id, len(serviceConfig))
		}

		// Register the created service using the shared helper
		if err := registerServiceInstance(ctx, id, serviceInstance); err != nil {
			return ctx, fmt.Errorf("failed to register mock service: %w", err)
		}

		// The testing.TB.Cleanup registered by SetupTest should handle
		// verifying expectations on mocks created with ctx.T().

		return ctx, nil
	}
}

// WithService creates a TestContextBuilderOption that wraps RegisterService
// to register and configure a Service for testing purposes.
func WithService(id string, factory core.ServiceFactory) TestContextBuilderOption {
	return func(tctx TestContext) (TestContext, error) {
		opts, err := RegisterService(tctx, id, factory)
		if err != nil {
			return tctx, err
		}
		return ProcessCtxOptions(tctx, opts...)
	}
}

// WithServiceFactory registers a real service by invoking the factory immediately,
// then registers the instance during startup. This lets returned context options
// be applied before startup.
func WithServiceFactory(id string, factory core.ServiceFactory) TestContextBuilderOption {
	return func(ctx TestContext) (TestContext, error) {

		// Create the service instance using the test context
		serviceInstance, ctxOpts, err := factory()
		if err != nil {
			return nil, fmt.Errorf("failed to create service '%s': %w", id, err)
		}

		// Ensure the service instance is a pointer so it can be modified by the wiring function.
		// If the factory returns a value type, we need to take its address so wiring works.
		servicePtr := reflect.ValueOf(serviceInstance)
		if servicePtr.Kind() != reflect.Ptr {
			servicePtr = reflect.New(servicePtr.Type())
			servicePtr.Elem().Set(reflect.ValueOf(serviceInstance))
			serviceInstance = servicePtr.Interface().(core.Service)
		}

		// Wire up the service's BaseComponent with context, logger, DB, and config
		startupOpt := core.ContextWithStartupComponent(serviceInstance)

		// Register a startup function that will register the service in the context
		registerOpt := core.ContextWithStartupFunc(func(coreCtx core.Context) error {
			tctx, ok := coreCtx.(TestContext)
			if !ok {
				return fmt.Errorf("context is not a TestContext, cannot use WithServiceFactory")
			}
			// Ensure the created instance is not nil
			if serviceInstance == nil {
				return fmt.Errorf("service factory for '%s' returned nil", id)
			}

			// Register the created service in the context
			if err := registerServiceInstance(tctx, id, serviceInstance); err != nil {
				return fmt.Errorf("failed to register service: %w", err)
			}

			return nil
		})

		// Apply wiring first, then registration, then other options
		options, err := ProcessCtxOptions(ctx, WrapCoreOption(startupOpt))
		if err != nil {
			return nil, err
		}

		options, err = ProcessCtxOptions(options, WrapCoreOption(registerOpt))
		if err != nil {
			return nil, err
		}

		options, err = ProcessCtxOptions(options, WrapCoreOptions(ctxOpts)...)
		if err != nil {
			return nil, err
		}

		return options, nil
	}
}

// WithCron creates a TestContextBuilderOption that enables cron service for testing.
func WithCron() TestContextBuilderOption {
	return CombineOptions(func(ctx TestContext) (TestContext, error) {
		EnableCron()
		ctx.T().Cleanup(func() {
			DisableCron()
		})
		return ctx, nil
	}, WithConfig("core.cron.enabled", true))
}

// WithCronService adds the real CronService to the test context and enables it in config.
func WithCronService() TestContextBuilderOption {
	return CombineOptions(
		WithConfig("core.cron.enabled", true),
		WithServiceFactory(core.CRON_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return service.NewCronService()
		}),
	)
}

// --- Modular Mock Service Setup Functions ---

// WithMockAccessService adds a mock AccessService to the test context.
func WithMockAccessService() TestContextBuilderOption {
	return WithMockService(core.ACCESS_SERVICE, func(tb TB, _ TestContext) any {
		return NewMockAccessService(tb)
	})
}

// WithMockAuthService adds a mock AuthService to the test context.
func WithMockAuthService() TestContextBuilderOption {
	return WithMockService(core.AUTH_SERVICE, func(_ TB, ctx TestContext) any {
		return NewMockAuthService(ctx)
	})
}

// WithMockContentScannerService adds a mock ContentScannerService to the test context.
func WithMockContentScannerService() TestContextBuilderOption {
	return WithMockService(core.CONTENT_SCANNER_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockContentScannerService(tb)
	})
}

// WithMockCronService adds a mock CronService to the test context.
// Note: CronService mock comes with pre-configured expectations for common operations
// to simplify test setup.
func WithMockCronService() TestContextBuilderOption {
	return WithMockService(core.CRON_SERVICE, func(tb TB, _ TestContext) any {
		_mock := mocks.NewMockCronService(tb)
		_mock.EXPECT().RegisterEntity(mock.MatchedBy(func(arg interface{}) bool {
			_, ok := arg.(core.Cronable)
			return ok
		})).Maybe().Return()
		return _mock
	})
}

// WithMockHTTPService adds a mock HTTPService to the test context.
func WithMockHTTPService() TestContextBuilderOption {
	return WithMockService(core.HTTP_SERVICE, func(tb TB, ctx TestContext) any {
		mockSvc := NewMockHTTPService(tb)
		mockSvc.router = ctx.Router()
		mockSvc.cmanager = ctx.Config()
		return mockSvc
	})
}

// WithMockHashMappingService adds a mock HashMappingService to the test context.
func WithMockHashMappingService() TestContextBuilderOption {
	return WithMockService(core.HASH_MAPPING_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockHashMappingService(tb)
	})
}

// WithMockMailerService adds a mock MailerService to the test context.
func WithMockMailerService() TestContextBuilderOption {
	return WithMockService(core.MAILER_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockMailerService(tb)
	})
}

// WithMockOTPService adds a mock OTPService to the test context.
func WithMockOTPService() TestContextBuilderOption {
	return WithMockService(core.OTP_SERVICE, func(_ TB, ctx TestContext) any {
		return NewMockOTPService(ctx)
	})
}

// WithMockPasswordResetService adds a mock PasswordResetService to the test context.
func WithMockPasswordResetService() TestContextBuilderOption {
	return WithMockService(core.PASSWORD_RESET_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockPasswordResetService(tb)
	})
}

// WithMockPinService adds a mock PinService to the test context.
func WithMockPinService() TestContextBuilderOption {
	return WithMockService(core.PIN_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockPinService(tb)
	})
}

// WithMockRequestService adds a mock RequestService to the test context.
func WithMockRequestService() TestContextBuilderOption {
	return WithMockService(core.REQUEST_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockRequestService(tb)
	})
}

// WithMockRenterService adds a mock RenterService to the test context.
func WithMockRenterService() TestContextBuilderOption {
	return WithMockService(core.RENTER_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockRenterService(tb)
	})
}

// WithStatefulMockRenterService adds a stateful mock RenterService to the test context.
func WithStatefulMockRenterService() TestContextBuilderOption {
	return WithMockService(core.RENTER_SERVICE, func(tb TB, _ TestContext) any {
		return NewMockRenterService(tb)
	})
}

// WithMockStorageService adds a mock StorageService to the test context.
func WithMockStorageService() TestContextBuilderOption {
	return WithMockService(core.STORAGE_SERVICE, func(tb TB, ctx TestContext) any {
		return NewMockStorageService(tb, ctx)
	})
}

// WithMockTUSService adds a mock TUSService to the test context.
func WithMockTUSService() TestContextBuilderOption {
	return WithMockService(core.TUS_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockTUSService(tb)
	})
}

// WithMockUserService adds a mock UserService to the test context.
func WithMockUserService() TestContextBuilderOption {
	return WithMockService(core.USER_SERVICE, func(_ TB, ctx TestContext) any {
		return NewMockUserService(ctx)
	})
}

// WithHTTPService adds the real HTTPService to the test context.
func WithHTTPService() TestContextBuilderOption {
	return WithServiceFactory(core.HTTP_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
		return service.NewHTTPService()
	})
}

// WithMockWorkflowService adds a mock WorkflowService to the test context.
func WithMockWorkflowService() TestContextBuilderOption {
	return WithMockService(core.WORKFLOW_SERVICE, func(tb TB, ctx TestContext) any {
		return NewMockWorkflowService(ctx)
	})
}

// WithMockUploadService adds a mock UploadService to the test context.
func WithMockUploadService() TestContextBuilderOption {
	return WithMockService(core.UPLOAD_SERVICE, func(tb TB, _ TestContext) any {
		return mocks.NewMockUploadService(tb)
	})
}

// getMock is a generic helper to retrieve and type assert mock services from the context
func getMock[T any](ctx core.Context, id string) *T {
	svc := ctx.Service(id)
	if svc == nil {
		panic(fmt.Sprintf("%s service not found in context", id))
	}

	mockSvc, ok := svc.(*T)
	if !ok {
		panic(fmt.Sprintf("%s service is not a mock - expected *%T, got %T", id, mockSvc, svc))
	}

	return mockSvc
}

// GetMockAccessService returns the mock access service from the context for testing
// Panics if the access service is not a mock
func GetMockAccessService(ctx core.Context) *MockAccessService {
	return getMock[MockAccessService](ctx, core.ACCESS_SERVICE)
}

// GetMockAuthService returns the mock auth service from the context for testing
// Panics if the auth service is not a mock
func GetMockAuthService(ctx core.Context) *MockAuthService {
	return getMock[MockAuthService](ctx, core.AUTH_SERVICE)
}

// GetMockContentScannerService returns the mock content scanner service from the context for testing
// Panics if the content scanner service is not a mock
func GetMockContentScannerService(ctx core.Context) *mocks.MockContentScannerService {
	return getMock[mocks.MockContentScannerService](ctx, core.CONTENT_SCANNER_SERVICE)
}

// GetMockCronService returns the mock cron service from the context for testing
// Panics if the cron service is not a mock
func GetMockCronService(ctx core.Context) *mocks.MockCronService {
	return getMock[mocks.MockCronService](ctx, core.CRON_SERVICE)
}

// GetMockHTTPService returns the mock http service from the context for testing
// Panics if the http service is not a mock
func GetMockHTTPService(ctx core.Context) *MockHTTPService {
	return getMock[MockHTTPService](ctx, core.HTTP_SERVICE)
}

// GetMockHashMappingService returns the mock hash mapping service from the context for testing
// Panics if the hash mapping service is not a mock
func GetMockHashMappingService(ctx core.Context) *mocks.MockHashMappingService {
	return getMock[mocks.MockHashMappingService](ctx, core.HASH_MAPPING_SERVICE)
}

// GetMockMailerService returns the mock mailer service from the context for testing
// Panics if the mailer service is not a mock
func GetMockMailerService(ctx core.Context) *mocks.MockMailerService {
	return getMock[mocks.MockMailerService](ctx, core.MAILER_SERVICE)
}

// GetMockOTPService returns the mock otp service from the context for testing
// Panics if the otp service is not a mock
func GetMockOTPService(ctx core.Context) *MockOTPService {
	return getMock[MockOTPService](ctx, core.OTP_SERVICE)
}

// GetMockPasswordResetService returns the mock password reset service from the context for testing
// Panics if the password reset service is not a mock
func GetMockPasswordResetService(ctx core.Context) *mocks.MockPasswordResetService {
	return getMock[mocks.MockPasswordResetService](ctx, core.PASSWORD_RESET_SERVICE)
}

// GetMockPinService returns the mock pin service from the context for testing
// Panics if the pin service is not a mock
func GetMockPinService(ctx core.Context) *mocks.MockPinService {
	return getMock[mocks.MockPinService](ctx, core.PIN_SERVICE)
}

// GetMockRequestService returns the mock request service from the context for testing
// Panics if the request service is not a mock
func GetMockRequestService(ctx core.Context) *mocks.MockRequestService {
	return getMock[mocks.MockRequestService](ctx, core.REQUEST_SERVICE)
}

// GetMockRenterService returns the mock renter service from the context for testing
// Panics if the renter service is not a mock
func GetMockRenterService(ctx core.Context) *mocks.MockRenterService {
	return getMock[mocks.MockRenterService](ctx, core.RENTER_SERVICE)
}

// GetMockStorageService returns the mock storage service from the context for testing
// Panics if the storage service is not a mock
func GetMockStorageService(ctx core.Context) *MockStorageService {
	return getMock[MockStorageService](ctx, core.STORAGE_SERVICE)
}

// GetMockTUSService returns the mock tus service from the context for testing
// Panics if the tus service is not a mock
func GetMockTUSService(ctx core.Context) *mocks.MockTUSService {
	return getMock[mocks.MockTUSService](ctx, core.TUS_SERVICE)
}

// GetMockUserService returns the mock user service from the context for testing
// Panics if the user service is not a mock
func GetMockUserService(ctx core.Context) *MockUserService {
	return getMock[MockUserService](ctx, core.USER_SERVICE)
}

// GetMockWorkflowService returns the mock workflow service from the context for testing
// Panics if the workflow service is not a mock
func GetMockWorkflowService(ctx core.Context) *MockWorkflowService {
	return getMock[MockWorkflowService](ctx, core.WORKFLOW_SERVICE)
}

// registerServiceInstance registers a service instance both locally in the test context
// and globally with the core framework.
func registerServiceInstance(ctx TestContext, id string, instance any, plugin ...string) error {
	if instance == nil {
		return fmt.Errorf("service instance for '%s' is nil", id)
	}

	// Register locally in test context
	ctx.RegisterService(id, instance)

	// Register globally if it implements core.Service
	if svc, ok := instance.(core.Service); ok {
		core.UnregisterService(id)
		core.RegisterService(core.ServiceInfo{
			ID: id,
			Factory: func() (core.Service, []core.ContextBuilderOption, error) {
				return svc, nil, nil
			},
		}, plugin...)
	} else {
		ctx.Logger().Warn("Service instance does not implement core.Service; global registration skipped",
			zap.String("service", id),
			zap.Any("type", reflect.TypeOf(instance)))
	}

	return nil
}

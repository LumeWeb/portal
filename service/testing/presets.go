package testing

import (
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service"
)

// PresetE2E - Full end-to-end test with real services, mock S3, and stateful mock renter
// This preset provides a realistic integration test environment with:
// - Real HTTP service for API testing
// - Real services for upload, pin, storage, request, workflow, user, TUS
// - Mock S3 storage service for safe testing without network calls
// - Stateful mock renter for renter interactions
// - Real cron service
// - Real access service
func PresetE2E() coreTesting.TestContextBuilderOption {
	// Service initialization order based on dependency graph:
	// Layer 1 (no declared deps): cron, mailer, request, upload, content scanner, hash mapping, renter
	// Layer 2 (requiring layer 1): user (depends on mailer + cron), tus (depends on request), workflow (depends on request + cron)
	// Layer 3 (requiring layer 2): otp (depends on user), storage (depends on renter + upload), pin (depends on upload)
	// Layer 4 (requiring layer 3): auth (depends on user + otp), password reset (depends on user + mailer)
	// Layer 5 (globally required, always initialized): access, http
	return coreTesting.CombineOptions(
		// Renter service - must be first as storage service depends on it
		coreTesting.WithStatefulMockRenterService(),

		// Layer 1: Foundation services
		coreTesting.WithServiceFactory(core.CRON_SERVICE, service.NewCronService),

		// Mailer requires a templateRegistry parameter
		coreTesting.WithServiceFactory(core.MAILER_SERVICE, func() (core.Service, []core.ContextBuilderOption, error) {
			return service.NewMailerService(service.NewMailerTemplateRegistry())
		}),

		coreTesting.WithServiceFactory(core.REQUEST_SERVICE, service.NewRequestService),
		coreTesting.WithServiceFactory(core.UPLOAD_SERVICE, service.NewMetadataService),
		coreTesting.WithServiceFactory(core.CONTENT_SCANNER_SERVICE, service.NewContentScannerService),
		coreTesting.WithServiceFactory(core.HASH_MAPPING_SERVICE, service.NewHashMappingService),

		// Layer 2: First dependency tier
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
		coreTesting.WithServiceFactory(core.TUS_SERVICE, service.NewTUSService),
		coreTesting.WithServiceFactory(core.WORKFLOW_SERVICE, service.NewWorkflowCoordinator),

		// Layer 3: Second dependency tier
		coreTesting.WithServiceFactory(core.OTP_SERVICE, service.NewOTPService),
		coreTesting.WithServiceFactory(core.STORAGE_SERVICE, service.NewStorageService),
		coreTesting.WithServiceFactory(core.PIN_SERVICE, service.NewPinService),

		// Layer 4: Third dependency tier
		coreTesting.WithServiceFactory(core.AUTH_SERVICE, service.NewAuthService),
		coreTesting.WithServiceFactory(core.PASSWORD_RESET_SERVICE, service.NewPasswordResetService),

		// Layer 5: Globally required services
		coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService),
		coreTesting.WithServiceFactory(core.HTTP_SERVICE, service.NewHTTPService),
	)
}

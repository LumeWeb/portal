# [0.1.0-develop.3](https://git.lumeweb.com/LumeWeb/portal/compare/v0.1.0-develop.2...v0.1.0-develop.3) (2023-09-09)

## 0.5.0 (2026-05-26)

### Breaking Changes

- Major changes

### Features

- implement migration system using dbmate
- add comprehensive testing utilities for core package
- add support for API extensions
- implement mockery-based mocks for core services
- add logger methods to the Context interface
- support benchmarks in test context
- add ParseStorageHash helper
- add NewStorageHashFromRawMultihash helper to simplify CID conversion when we have no other data
- add GetScanResultById to ContentScannerService
- add composite WorkflowService interface
- add RegisteredScanners to ContentScannerService
- implement modular hash parsing system
- add GetServiceConfig, GetAPIConfig, GetProtocolConfig generics helpers
- Add web bundle support and refactor API routing
- migrate from gorilla/mux to echo framework
- add schema mapping support for API types
- add JSON tags to AccountError struct and update dependencies
- add InitContext, ProcessStartupFuncs, ProcessExitFuncs
- add GetMockConfig helper for testing
- add RegisterEvents testing helper for event registration
- add mock access service and default test context options
- enhance mock HTTP service and test context with router support
- add config manager support to MockHTTPService
- add FromUUID helper to convert raw UUID to BinaryUUID
- enhance test database utilities and context management
- add test utilities and environment setup helpers
- add test helpers WithDBAndOptions, WithDBNoMigrationsAndOptions, WithOptions
- improve documentation and add mock service factory support
- add config mocking helpers and documentation
- enhance test coverage and mock generation
- add reset functions and improve testing capabilities
- globally solve CORS for root and plugin apis
- add WithSQLitePluginMigrations test helper
- add boot complete event control to test context
- add WithMockUploadService option function
- implement strongly-typed event system
- refactor config manager and validation
- refactor config management and migration system
- complete architectural overhaul of cron service
- enhance service implementations and add comprehensive testing
- add mock protocol support and refactor protocol registration
- add ConfigBuilder for flexible test configuration
- add CombineOptions for TestContextBuilder
- add retry policy support and workflow cron integration
- enhance workflow execution and request handling
- add NewScanOperation helper function
- enhance workflow options with koanf configuration
- add OperationHelper interface and default implementation
- add Protocol support to OperationHelper
- add StorageHash method to OperationHelper interface
- add core testing utilities for operations, requests and workflows
- add Context getter to OperationHelper interface
- add Logger method to OperationHelper interface
- add StructuredWorkflowData method to OperationHelper
- add StartWorkflow method to OperationHelper
- add workflow data update methods and caching
- add request conversion and foreground execution
- add S3 mock support and helper functions
- add operation name helper functions
- add workflows support to Protocol interface
- add autoTriggerFirstStep to workflow registration
- add helper methods for workflow testing
- add workflow enable/disable functionality
- add tracking for configured plugins and services
- add options pattern for download operations
- add enhanced MockRenterService implementation
- add status message to request status updates
- add reader passthrough functionality
- add Bytes() method to StorageHash interface
- add TUS upload operation support
- add methods for temporary upload paths and update S3 handling
- add NewError wrapper for global error registry
- add cron service support and fix initialization
- enhance workflow instance querying and filtering
- enhance job validation and refactor core logic
- add WithUnregisterPlugin test helper
- add WithUnregisterService test context option
- add workflow support to plugins and operation caching
- add S3 upload, download and delete operations
- add S3Exists method to check object existence
- add dynamic global path registration
- add environment variable configuration helpers
- add HTTP service support to test framework
- add Port() method to HTTPService interface and implementations
- add WithPlugins option for registering multiple test plugins
- add secure config property
- add error namespace import/export functionality
- support cron component in DefaultTestContextOptions
- add WithCronService helper function
- add EmptyUUID preset
- add BinaryUUID Equals and Empty
- add comprehensive boot and init event system
- add workflow instance management and enhance testing infrastructure
- add message field to workflow status
- add RegisterGlobalPath method to MockHTTPService
- add PreFinishResponse callback and CID generation
- add distinct filter listing functionality
- add human-readable display names for operations and statuses
- add DisplayName() to Protocol interface
- add support for post upload operations
- add PostUploadOperationName function
- add support for firing boot events and handling function options in test context
- add custom name support to operations
- add CompleteWorkflowStep helper method
- extract upload retrieval logic and add metadata getter
- add configurable S3 temporary upload ID
- add S3TemporaryUploadExists method
- add support for configuring mock services with Config() expectation
- add upload completed event and handler
- add optional service retrieval and execution helpers
- add quota exceeded error definitions and checker
- add user ID to download completed event
- add GetPinByHash method to PinService interface and implementation
- refactor test context initialization and option processing
- add JWT helper and mock services for authentication testing
- enhance mock auth service with JWT helper and improve mock expectations
- enhance S3 operations with ReadSeekCloser support
- add input validation and context cancellation to S3Reader
- implement ReadSeekCloser for TUS reading
- enhance config handling and mock plugin capabilities
- migrate method signatures to context.Context and add instrumentation
- implement context extraction and tracing for events and refactor otel helpers
- replace OTLP exporter with uptrace-go
- sanitize binary UUIDs in SQL traces to prevent UTF-8 errors
- add workflow configuration helpers for DRY plugin development
- add real-time progress tracking for upload hashing operations
- add DetachContext for long-running workflows
- add progress tracking framework for operations
- add JSONSchema method to BinaryUUID for swagger generation
- add debug logging for email operations
- add logging for config creation failures
- add DNS resolver override support
- add custom DNS resolver support for mail client
- add custom DNS resolver port support
- add TLS policy configuration support
- validate auth_type against SMTPAuthType constants
- add E2E preset for integration tests
- refactor lookup methods with generic helpers
- enhance panic handling with structured stack trace capture
- Add operation tracking fields to upload/download events
- add GetAllPinsByHash to PinService
- add io.ReaderAt support to TUSUploadReader
- add per-job-type metrics and OTel tracing
- add optional basic auth to metrics endpoint

### Fixes

- BaseEndpoint must have a protocol
- don't act in cluster mode if sia cluster setting is false
- BuildInfo API needs to return the instance variables not the global
- always merge in config to m.config
- improve handling of build info such as commits or time
- handle plugin.Version being nil
- Remove duplicate event recorder implementation from testing.go
- Remove duplicate event recorder implementation
- cleanup imports
- wrong casing for NewLogger
- add missing registration for ContentScannerService and add type check
- add missing String method to MockStorageHash and add type check
- StorageHashDefault.String needs to call Multihash B58String, not String as String is an alias to HexString
- move WORKFLOW_SERVICE to core
- fix WORKFLOW_SERVICE namespace
- ContentScannerServiceDefault GetScanResults/GetScanResultById needs to return []*core.ScanResult
- APIExtensions needs to pass the Context
- handle new api extension structure
- assign Files field before applying options
- defer file close in getProcessedManifest
- Correct inverted bounds check for web bundle index
- Sanitize paths in bundle file system to prevent traversal
- add security validation for web bundle paths and plugin IDs
- Use f.size instead of non-existent f.content in Seek
- improve file handling in BundleFileSystem
- remove duplicate manifest route and update middleware dependency
- Replace deprecated filepath.HasPrefix with strings.HasPrefix
- add nil check for web bundle in file server handler
- update portal-router and gswagger dependencies
- bad routing variables
- malformed bundle routing paths
- improve path handling and security in web bundle FS
- correct event type and context option wrapping in RegisterEvents
- need to use core.GetEvents()
- cleanup MockHTTPService by removing unused imports and generalizing test interface
- ensure mock HTTP service includes router
- update MockHTTPService to use config.Manager interface
- correct UUID handling in CronJob model and add tests
- move TestingShutdown to testing.go as ShutdownTestContext
- relax MockServiceFactory type constraint and add service validation
- add mutex to serialize test execution
- reset event registry before registering
- clean up test context option handling
- Use Unlock after Lock in ClearTestCaseContextOptions
- Correctly register service in testing helper
- ensure DB migrations properly enable mock DB
- configure mock HTTP service to return test context router
- update dependencies and improve type naming
- preserve package-level DB flags in ResetAllState
- reorganize imports and fix plugin registration
- remove unused fields from requests table schema
- missing fmt import
- add autoTriggerFirstStep parameter to RegisterWorkflow
- remove indexes for columns that no longer exist
- correct metadata JSON handling and type usage
- add mock expectations for config manager getters
- properly load namespaces in mock config manager
- correct plugin configuration key formats
- WorkflowTest.AssertOperationStatusProgress should take a float
- add Bytes() method to MockStorageHash
- wrong s3 config keys paths
- update TUSDefaultUploadCreatedHandler wrapper arguments
- add protocol parameter to TUS handler methods
- simplify TUS handler and remove unused import
- fix workflow handling in TUS upload completion
- ensure proper status propagation in request lifecycle
- move cron service initialization to proper location
- improve argument handling and type safety
- use go-base32 for multibase encoding/decoding
- move RegisterServicesFromPlugins call earlier in BootEnvironment
- ensure ConfigurePluginServices runs before ProcessStartupFuncs
- bypass service dependency checks in ConfigurePluginServices
- fix context initialization flow
- event inner types arent passed as a pointer
- plugin workflow registration must come after setup
- change fatal errors to non-fatal in config getters
- correct context processing in PortalImpl.Init()
- cant use ctx Fire as it looses type info
- we can't do a read lock inside a read lock
- remove duplicate imports
- enhance database constraint violation error handling
- add schedule_type column to MySQL cron_jobs table definition
- remove unused variables and update mock provider type
- update mocks
- fix WithEnvConfigOrDefault arguments
- fix WithEnvConfigOrDefault arguments
- only call coordinator close if cron is enabled
- add delay before port config update and ensure HTTP service wait
- add nil check
- correct return type in RegisterService error cases
- remove service registration from WithPlugins to use RegisterServicesFromPlugins
- Add nil check for API extension in RegisterAPIExtensions
- use exported core functions in WithErrorNamespaces
- standardize on BootCompleted naming in event system
- update boot complete event constants in testing context
- correct event package reference in BootEnvironment
- correct API domain resolution logic
- resolve undefined size variable in TUS upload error logging
- imports
- standardize host/port handling in test API routes
- add port config when using mock HTTP service
- correct handler type assertion and add DisplayName mocks
- implement DisplayName in MockProtocol
- automatically advance workflow step after successful execution
- use local state variable in CleanupJob
- only remove one-shot jobs from scheduler after completion
- update request hash and CID type correctly
- prevent duplicate step completion and handle continue-on-failure behavior
- continue workflow execution on step failure
- handle nil upload in getUploadByIdentifier
- prevent path traversal in upload IDs
- improve upload ID validation error message
- set upload ID after saving existing upload
- handle errors when restoring error namespaces
- add error handling to service configuration
- Ensure configuration is loaded before startup functions
- WithMockS3 needs to use GetRealConfig
- handle missing On() support and validate serviceConfig length in mock setup
- use temporary directory path in test config
- remove duplicate source registration
- handle service type mismatches in optional lookup and callback execution
- handle nil error in FailWorkflowStep
- prevent duplicate startup function execution
- correct mock storage service initialization error message
- correct mock storage service variable naming
- simplify mock workflow option count
- use higher-layer workflow mock in test context
- enhance S3 reader seek and read behavior
- prevent negative discard bytes in S3 reader seek
- handle errors from S3Reader creation
- preserve original error for logging and retry decisions
- simplify TUS upload reader logic
- properly manage startup functions during migration setup
- correct type assignment check in mock plugin builder
- add type assertion check for mock service instances
- correct mock service config return value
- improve default value comparison logic
- Use event context for service calls
- improve error handling in account deletion job
- simplify DSN validation and logging setup
- improve OTEL log level handling
- reuse existing logger's OTEL level filter for consistency
- improve OTEL level filter management
- improve S3 upload error handling and tracking
- improve error handling in S3 multipart upload
- correct GORM transaction handling in S3 multipart upload
- handle upload errors correctly in S3 multipart upload
- simplify S3 multipart upload error handling
- handle S3 upload cleanup error gracefully
- properly wire up service with ContextWithStartupComponent
- resolve multiple issues in testing utilities
- add testify mock import to API extension test helper
- fix missing dot separator in ApplyConfig full key generation
- handle empty prefix in ApplyConfig to avoid leading dot
- change GetMockWorkflowService to return high-level wrapper type
- prepend ContextWithStartupComponent in RegisterService for proper wiring
- ensure ContextWithStartupComponent is applied to API extensions
- update SetPath calls to use string slice syntax
- initialize OperationFinder during startup
- remove unused import and variable in HashProgressReader
- add nil check before incrementing error counter in MetricTrackResult
- use service context instead of canceled hook context and round progress percentage
- update mapstructure to upstream v2.5.0
- fix progress tracking and prevent runtime panics
- improve status reporting for active operations
- preserve terminal states in status state management
- create temporary logger for config errors
- disable config validation for config-env command
- validate CONFIG_PATHS as directories and update defaults
- prevent malformed FQDN when mock API returns empty subdomain
- prevent nil pointer dereference when config manager is not set
- make registration more resilient to email failures
- fallback to next path when config creation fails
- properly format IPv6 addresses and respect network parameter
- address PR review feedback
- move DNS resolver setup after config init
- address PR review feedback
- implement custom DNS resolution with monkey-patching
- address race conditions, resource leaks, and test flakiness
- address PR review feedback
- handle empty TLS policy and extract shared constants
- remove ToUpper conversion on auth_type comparison
- add 'none' alias for SMTP auth type
- use keyMatch2 matcher for dynamic route patterns
- add custom key matcher for Echo's colon syntax
- address PR review feedback
- register keyMatchEcho before enforcer is used
- apply service config to regular services in MockPluginBuilder
- strip plugin name prefix from service IDs
- use dot separator in StripPluginNamePrefix
- correct mailer service factory return type
- move renter mock to first position ensure storage service dependency
- add mock S3 service after renter for safe testing
- resolve mock helper footgun causing ignored test expectations
- detach context in long-running workflows
- separate lifecycle management from operation context
- block worker to prevent immediate shutdown
- override ConfigDir and ConfigFile methods to return ManagerDefault paths
- wire plugin cron job registration into boot sequence
- move plugin cron registration to CronService Start()
- use c.Logger() instead of c.logger to avoid nil pointer
- add explicit jobType field to BaseCronJob
- use functional options pattern for BaseCronJob constructor
- fix context cancellation, IPv6 lookups, and fallback behavior
- prevent infinite recursion in custom DNS lookup functions
- remove default resolver fallbacks to prevent infinite recursion
- set subdomain on mockAPI in WithAPIExtension
- reorder component type switch cases
- correct variable name in registerAPIExtensions causing panics
- display structured panic info in error logs
- enhance migration error logging with database-specific details
- apply config values atomically before validation in ApplyConfig
- resolve all runtime test failures and break cron import cycle
- fix SetHeartbeat race and handle completed jobs gracefully
- stop heartbeat before Completed early return in CleanupJob
- follow NextRequestID chain in GetWorkflowStatus/Instance/StepInfo
- make StartOperationWorkflow idempotent by reusing registered workflows
- use constant-time comparison for metrics basic auth

## 0.3.2

### Patch Changes

- fd503f1: dont pass nil as a second argument

## 0.3.1

### Patch Changes

- 499b46a: call AddPluginWithBuild if we have Version set

## 0.3.0

### Minor Changes

- 1347612: ---

  # Release Notes

  ## Major Features

  - **Host Scanner Integration** 🔍
    - Implemented host scanner with hostscore.info API integration
    - Added advanced host scanning and price optimization capabilities
    - Introduced retry support on host scanning operations

  ## Renter System Improvements

  - **Host Management**
    - Added new Hosts method for direct usable host querying
    - Improved host scanning and autopilot testing

  ## System Enhancements

  - **Authentication**

    - Enhanced JWT token validation with ED25519

  - **Build System**

    - Added build information system
    - Restructured build info implementation
    - Externalized platform detection
    - Removed build host

  - **Cron Jobs**
    - Added support for one-off jobs without interfering with primary jobs
    - Improved one-off job handling and cleanup mechanism

  ## Dependencies Updates

  - Upgraded various dependencies:
    - github.com/gabriel-vasile/mimetype to 1.4.6
    - github.com/casbin/gorm-adapter/v3 to 3.29.0
    - go.uber.org/zap/exp to 0.3.0

## 0.2.1

### Patch Changes

- 3f63e61: ---
  - Update release workflow concurrency and triggers
  - Fix pointer dereferencing issue
  - Fix incorrect index name in TUS implementation

## 0.2.0

### Minor Changes

- 6b3aa17: Major Features & Improvements:

  Architecture & Core:

  - Implemented unified request-based architecture for better request handling and processing
  - Added support for cluster-aware configuration management with etcd
  - Introduced new configuration structure with improved validation and management
  - Implemented access control service and middleware with role-based permissions
  - Added comprehensive event system

  Storage & Upload Management:

  - Revamped upload process with improved multi-part upload handling
  - Added temporary upload functionality with S3 integration
  - Implemented TUS protocol support

  Database & Performance:

  - Improved database operations with retry mechanisms
  - Enhanced cron job reliability and error handling
  - Optimized price tracking and database compatibility

  Security & User Management:

  - Added account verification system
  - Implemented account deletion functionality
  - Enhanced cookie handling and session management
  - Improved password reset and email verification processes

## 0.1.1

### Patch Changes

- 4322c17: - CI/CD changes related to changesets and release workflow
  - Added functionality for handling custom events and storage object uploads in the core module
  - Fixed issues with array manipulation and cron service in the portal module
  - Added helper functions for configuration and protocol handling
  - Refactored HTTP startup logic into a dedicated init method

## 0.1.0

### Minor Changes

- 7d66329: New minimal, modular portal architecture. Portal should now always be built using `xportal`. No modules are included by default

### Bug Fixes

- handle failure on verifying token ([a06b79a](https://git.lumeweb.com/LumeWeb/portal/commit/a06b79a537f08d741faeb8319d558c9e64977c4b))

# [0.1.0-develop.2](https://git.lumeweb.com/LumeWeb/portal/compare/v0.1.0-develop.1...v0.1.0-develop.2) (2023-08-15)

### Bug Fixes

- need to change dnslink route registration to use a path param based route ([ae071a3](https://git.lumeweb.com/LumeWeb/portal/commit/ae071a30ecaa62ff431878c71a54059e3d3ce8b7))
- need to string off forward slash at beginning to match manifest file paths ([2f64f18](https://git.lumeweb.com/LumeWeb/portal/commit/2f64f18e24fa1e4ddd74ed6a8d2d44e483fff1dc))

# [0.1.0-develop.1](https://git.lumeweb.com/LumeWeb/portal/compare/v0.0.1...v0.1.0-develop.1) (2023-08-15)

### Bug Fixes

- abort if we don't have a password for the account, assume its pubkey only ([c20dec0](https://git.lumeweb.com/LumeWeb/portal/commit/c20dec020437d91cf2728852b8bed5c4a0c481e9))
- add a check for a 500 error ([df08fc9](https://git.lumeweb.com/LumeWeb/portal/commit/df08fc980ac3f710a67bd692b8126eb978699d5b))
- add missing request connection close ([dff3ca4](https://git.lumeweb.com/LumeWeb/portal/commit/dff3ca45895095b82ba2e76b2e61487e28151b7d))
- add shutdown signal and flag for renterd ([fb65690](https://git.lumeweb.com/LumeWeb/portal/commit/fb65690abd5c190dce30d3cfe0d079b27040a309))
- **auth:** eager load the account relation to return it ([a23d165](https://git.lumeweb.com/LumeWeb/portal/commit/a23d165caa3ba4832c9d37a0b833b9b58df60732))
- change jwtKey to ed25519.PrivateKey ([bf576df](https://git.lumeweb.com/LumeWeb/portal/commit/bf576dfaeef51078d7bdae885550fc235d49c1eb))
- close db on shutdown ([78ee15c](https://git.lumeweb.com/LumeWeb/portal/commit/78ee15cf4b5d3a55209a9c7559700a2c5b227f87))
- Ctx must be public ([a0d747f](https://git.lumeweb.com/LumeWeb/portal/commit/a0d747fdf4e6ee3fa6a3b4dca180e4f14af30ed9))
- ctx needs to be public in AuthService ([a3cfeba](https://git.lumeweb.com/LumeWeb/portal/commit/a3cfebab307a87bc895d7b1c1f0e6632a708562c))
- **db:** need to set charset, parseTime and loc in connection for mysql ([5d15ca3](https://git.lumeweb.com/LumeWeb/portal/commit/5d15ca330abd26576ef9865c110975aeb27c3ab3))
- disable client warnings ([9b8cb38](https://git.lumeweb.com/LumeWeb/portal/commit/9b8cb38496541b0ab50d28eef63658f9723c5802))
- dont try to stream if we have an error ([b21a425](https://git.lumeweb.com/LumeWeb/portal/commit/b21a425e24f5543802e7267369f37967d4805697))
- encode size as uint64 to the end of the cid ([5aca66d](https://git.lumeweb.com/LumeWeb/portal/commit/5aca66d91981d8fae88194df6b03c239dbd179a8))
- ensure all models auto increment the id field ([934f8e6](https://git.lumeweb.com/LumeWeb/portal/commit/934f8e6236ef1eef8db1d06a1d7a7fded8afe694))
- ensure we store the pubkey in lowercase ([def1b50](https://git.lumeweb.com/LumeWeb/portal/commit/def1b50cfcba8c68f3b95209790418638374fad9))
- handle duplicate tus uploads by hash ([f3172b0](https://git.lumeweb.com/LumeWeb/portal/commit/f3172b0d31f844b95a0e64b3a5d821f71b0fbe07))
- hasher needs the size set to 32 ([294370d](https://git.lumeweb.com/LumeWeb/portal/commit/294370d88dd159ae173a6a955a417a1547de60ed))
- if upload status code isn't 200, make it an err based on the body ([039a4a3](https://git.lumeweb.com/LumeWeb/portal/commit/039a4a33547a59b4f3ec86199664b5bb94d258a6))
- if uploading returns a 500 and its a slab error, treat as a 404 ([6ddef03](https://git.lumeweb.com/LumeWeb/portal/commit/6ddef03790971e346fa0a7d33a462f39348bc6cc))
- if we have an existing upload, just return it as if successful ([90170e5](https://git.lumeweb.com/LumeWeb/portal/commit/90170e5b81831f3d768291fd37c7c13e32d522fe))
- iris context.User needs to be embedded in our User struct for type checking to properly work ([1cfc222](https://git.lumeweb.com/LumeWeb/portal/commit/1cfc2223a6df614f26fd0337ced68d92e774589f))
- just use the any route ([e100429](https://git.lumeweb.com/LumeWeb/portal/commit/e100429b60e783f6c7c3ddecab7bb9b4dd599726))
- load awsConfig before db ([58165e0](https://git.lumeweb.com/LumeWeb/portal/commit/58165e01af9f2b183d654d3d8809cbd1eda0a9bb))
- make an attempt to look for the token before adding to db ([f11b285](https://git.lumeweb.com/LumeWeb/portal/commit/f11b285d4e255c1c4c95f6ac15aa904d7a5730e4))
- missing setting SetTusComposer ([80561f8](https://git.lumeweb.com/LumeWeb/portal/commit/80561f89e92dfa86887ada8361e0046ee6288234))
- newer gorm version causes db rebuilds every boot ([72255eb](https://git.lumeweb.com/LumeWeb/portal/commit/72255eb3c50892aa5f2cfdc4cb1daa5883f0affc))
- only panic if the error is other than a missing awsConfig file ([6e0ec8a](https://git.lumeweb.com/LumeWeb/portal/commit/6e0ec8aaf90e86bcb7cb6c8c53f6569e6885e0aa))
- output error info ([cfa7ceb](https://git.lumeweb.com/LumeWeb/portal/commit/cfa7ceb2f422a6e594a424315c8eaeffc6572926))
- PostPubkeyChallenge should be lowercasing the pubkey for consistency ([d680f06](https://git.lumeweb.com/LumeWeb/portal/commit/d680f0660f910e323356a1169ee13ef2e647a015))
- PostPubkeyChallenge should be using ChallengeRequest ([36745bb](https://git.lumeweb.com/LumeWeb/portal/commit/36745bb55b1d7cd464b085e410333089504591c1))
- PostPubkeyChallenge should not be checking email, but pubkey ([db3ba1f](https://git.lumeweb.com/LumeWeb/portal/commit/db3ba1f0148b6abc34b4606f9b8103963a3c6850))
- PostPubkeyLogin should be lowercasing the pubkey and signature ([09d53ff](https://git.lumeweb.com/LumeWeb/portal/commit/09d53ffa7645b64aed4170e698b8eb62d2c3590e))
- PostPubkeyLogin should not preload any model ([27e7ea7](https://git.lumeweb.com/LumeWeb/portal/commit/27e7ea7d7a0bbf6c147ff625591acf6376c6c62d))
- properly handle missing size bytes ([c0df04d](https://git.lumeweb.com/LumeWeb/portal/commit/c0df04d7d5309e32348ceecc68eecd64c5e5cba4))
- public_key should be pubkey ([09b9f19](https://git.lumeweb.com/LumeWeb/portal/commit/09b9f195f47ea9ae47069a517a77609c74ea3ca5))
- register LoginSession model ([48164ec](https://git.lumeweb.com/LumeWeb/portal/commit/48164ec320c693937ead352246ec1e94bede3684))
- register request validation ([c197b14](https://git.lumeweb.com/LumeWeb/portal/commit/c197b1425bbd689e8f662846de0478aff8d38f35))
- remove PrivateKey, rename PublicKey in Key model ([00f2b96](https://git.lumeweb.com/LumeWeb/portal/commit/00f2b962a0da956f971dc94d75726c1bab693232))
- rewrite gorm query logic for tus uploads ([f8aaeff](https://git.lumeweb.com/LumeWeb/portal/commit/f8aaeff6de2dc5e5321840460d55d79ad1b5ab1a))
- rewrite sql logic ([ce1b5e3](https://git.lumeweb.com/LumeWeb/portal/commit/ce1b5e31d5d6a69dc91d88a6fd2f1317e07dc1ea))
- rewrite streaming logic and centralize in a helper function ([bb26cfc](https://git.lumeweb.com/LumeWeb/portal/commit/bb26cfca5b4017bbbbf5aeee9bd3577c724f83ca))
- save upload info after every chunk ([038d2c4](https://git.lumeweb.com/LumeWeb/portal/commit/038d2c440b24b7c0f1ea72e0bfeda369f766c691))
- temp workaround on race condition ([e2db880](https://git.lumeweb.com/LumeWeb/portal/commit/e2db880038f51e0e16ce270fe29fce7785cce878))
- **tus:** switch to normal clone package, not generic ([faaec64](https://git.lumeweb.com/LumeWeb/portal/commit/faaec649ead00567ced56edfa9db11eb34655178))
- update default flag values ([241db4d](https://git.lumeweb.com/LumeWeb/portal/commit/241db4deb6808d950d55efa38e11d60469cc6778))
- update model relationships ([628f1b4](https://git.lumeweb.com/LumeWeb/portal/commit/628f1b4acaac1d2bf373b7008f2e0c070fd64ae5))
- **upload:** add account to upload record ([e018a4b](https://git.lumeweb.com/LumeWeb/portal/commit/e018a4b7430bc375ff3b72537e71295cdf67ef93))
- uploading of main file ([7aea462](https://git.lumeweb.com/LumeWeb/portal/commit/7aea462ab752e999030837d13733508369524cf3))
- upstream renterd updates ([5ad91ad](https://git.lumeweb.com/LumeWeb/portal/commit/5ad91ad263f01830623958141a7e7c8523bee85f))
- use AccountID not Account ([f5e4377](https://git.lumeweb.com/LumeWeb/portal/commit/f5e437777a52e2a9bbf55903cea17ec073fbb406))
- use bufio reader ([90e4ce6](https://git.lumeweb.com/LumeWeb/portal/commit/90e4ce6408391dc270ca4405a7c5282c2d4766b2))
- use challengeObj ([9b82fa7](https://git.lumeweb.com/LumeWeb/portal/commit/9b82fa7828946803289add03fc84be1dc4f86d8b))
- use database.path over database.name ([25c7d6d](https://git.lumeweb.com/LumeWeb/portal/commit/25c7d6d4fb48b69239eba131232a78e90a576e2f))
- use getWorkerObjectUrl ([4ff1334](https://git.lumeweb.com/LumeWeb/portal/commit/4ff1334d8afd9379db687fc6b764f5b0f1bcc08c))
- Use gorm save, and return nil if successful ([26042b6](https://git.lumeweb.com/LumeWeb/portal/commit/26042b62acd7f7346f1a99a0ac37b3f2f99e3f75))
- we can't use AddHandler inside BeginRequest ([f941ee4](https://git.lumeweb.com/LumeWeb/portal/commit/f941ee46d469a3f0a6302b188f566029fdec4e70))
- wrap Register api in an atomic transaction to avoid dead locks ([e09e51b](https://git.lumeweb.com/LumeWeb/portal/commit/e09e51bb52d513abcbbf53352a5d8ff68eb5364a))
- wrong algo ([86380c7](https://git.lumeweb.com/LumeWeb/portal/commit/86380c7b3a97e785b99af456305c01d18f776ddf))

### Features

- add a status endpoint and move cid validation to a utility method ([38b7615](https://git.lumeweb.com/LumeWeb/portal/commit/38b76155af954dc3602a5035cb7b53a7f625fbfd))
- add a Status method for uploads ([1f195cf](https://git.lumeweb.com/LumeWeb/portal/commit/1f195cf328ee176be9283ab0cc40e65bb6c40948))
- add auth status endpoint ([1dd4fa2](https://git.lumeweb.com/LumeWeb/portal/commit/1dd4fa22cdfc749c5474f94108bca0aec34aea81))
- add bao package and rust bao wasm library ([4c649bf](https://git.lumeweb.com/LumeWeb/portal/commit/4c649bfcb92e8632e45cf10b27fa062ff1680c32))
- add cid package ([706f7a0](https://git.lumeweb.com/LumeWeb/portal/commit/706f7a05b9a4ed464f693941235aa7e9ca14145a))
- add ComputeFile bao RPC method ([687f26c](https://git.lumeweb.com/LumeWeb/portal/commit/687f26cc779f4f50166108d6e78fe1456cfa128d))
- add debug mode logging support ([99d7b83](https://git.lumeweb.com/LumeWeb/portal/commit/99d7b8347af25fe65a1f1aecc9960424a101c279))
- add download endpoint ([79fd550](https://git.lumeweb.com/LumeWeb/portal/commit/79fd550c54bf74e84d012805f60c036c19fbbef2))
- add EncodeString function ([488f873](https://git.lumeweb.com/LumeWeb/portal/commit/488f8737c09b7757c5649b3d8a3568e3c1d5fe45))
- add files service with upload endpoint ([b16beeb](https://git.lumeweb.com/LumeWeb/portal/commit/b16beebabb254488897edde870e9588b7be5293e))
- add files/upload/limit endpoint ([b77bebe](https://git.lumeweb.com/LumeWeb/portal/commit/b77bebe3b1a03cecdd7e80f575452d5ce91ccfac))
- add getCurrentUserId helper function ([29d6db2](https://git.lumeweb.com/LumeWeb/portal/commit/29d6db20096e61efa9a792ef837ef93ca14107ae))
- add global cors ([1f5a3d1](https://git.lumeweb.com/LumeWeb/portal/commit/1f5a3d19e44f1db2f8587623e868fa48b23d1a74))
- add jwt package ([ea99108](https://git.lumeweb.com/LumeWeb/portal/commit/ea991083276a576003eb3633bd1bde98e13dfe84))
- add more validation, and put account creation, with optional pubkey in a transaction ([699e424](https://git.lumeweb.com/LumeWeb/portal/commit/699e4244e0d877d8d9df9d3d4894351785fe7f4d))
- add new user service object that implements iris context User interface ([a14dad4](https://git.lumeweb.com/LumeWeb/portal/commit/a14dad43ed3140f73d817ef2438aacbc0939de69))
- add newrelic support ([06b3ab8](https://git.lumeweb.com/LumeWeb/portal/commit/06b3ab87f7e1b982d3fb42a3e06897a2fd1387ed))
- add pin model ([aaa2c17](https://git.lumeweb.com/LumeWeb/portal/commit/aaa2c17212bd5e646036252a0e1f8d8bdb68f5a7))
- add pin service method ([8692a02](https://git.lumeweb.com/LumeWeb/portal/commit/8692a0225ebb71502811cba063e32dd11cdd10c9))
- add PostPinBy controller endpoint for pinning a file ([be03a6c](https://git.lumeweb.com/LumeWeb/portal/commit/be03a6c6867f305529af90e6206a0597bb84f015))
- add pprof support ([ee17409](https://git.lumeweb.com/LumeWeb/portal/commit/ee17409e1252e9cbae0b17ccbb1949c9a81dff82))
- add proof download ([3b1e860](https://git.lumeweb.com/LumeWeb/portal/commit/3b1e860256297d3515f0fcd58dd28292c316d79f))
- add StringHash ([118c679](https://git.lumeweb.com/LumeWeb/portal/commit/118c679f769bec2971e4e4b00ec41841a02b8a1c))
- add swagger support ([49c3844](https://git.lumeweb.com/LumeWeb/portal/commit/49c38444066c89d7258fd85d114d9d74babb8d55))
- add upload model ([f73a04b](https://git.lumeweb.com/LumeWeb/portal/commit/f73a04bb2e48b78e22b531a9121fe4baa011deaf))
- add Valid, and Decode methods, and create CID struct ([4e6c29f](https://git.lumeweb.com/LumeWeb/portal/commit/4e6c29f1fd7c33ce442fe741e08b32c8e3e9f393))
- add validation to account register ([7257b5d](https://git.lumeweb.com/LumeWeb/portal/commit/7257b5d597a28069c87437cabd71f51c187eb80c))
- generate and/or load an ed25519 private key for jwt token generation ([85a0295](https://git.lumeweb.com/LumeWeb/portal/commit/85a02952dffb1873c557f30483606d678e46749d))
- initial dnslink support ([cd2f63e](https://git.lumeweb.com/LumeWeb/portal/commit/cd2f63eb72c2bfc404d8d1b5a6fdb53f61a31d1b))
- pin file after basic upload ([892f093](https://git.lumeweb.com/LumeWeb/portal/commit/892f093d93348459d113041104d773fdd5124a8d))
- pin file after tus upload ([5579ab8](https://git.lumeweb.com/LumeWeb/portal/commit/5579ab85a374be457163d06caf1ac6e260082cca))
- tus support ([3005be6](https://git.lumeweb.com/LumeWeb/portal/commit/3005be6fec8136214c1e9480c788f62564a2c5f9))
- wip version ([9a4c3d5](https://git.lumeweb.com/LumeWeb/portal/commit/9a4c3d5d13a3e76fe91eb5d78a6f2f0f8e238f80))

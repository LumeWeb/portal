package example_api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"go.lumeweb.com/portal-middleware/auth/jwt"
	mcontext "go.lumeweb.com/portal-middleware/context"
	"go.lumeweb.com/portal-middleware/middleware"
	router "go.lumeweb.com/portal-router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
)

func TestAPIEndpoints(t *testing.T) {
	// Use RunTestCaseWithDB for per-test context setup with database support.
	// The test logic is provided as the second argument (a function).
	// The API registration option is passed as a variadic option after the test function.
	coreTesting.RunTestCase(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		// The API is now registered and configured within this test context
		// because NewAPIRegistrationOption was passed as an option to RunTestCaseWithDB.

		t.Run("public hello endpoint", func(t *testing.T) {
			//t.Parallel() // Subtests can run in parallel

			req := httptest.NewRequest(http.MethodGet, "/hello", nil)
			req.Host = ctx.Config().Config().Core.Domain
			rec := httptest.NewRecorder()

			ctx.Router().ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "text/plain; charset=UTF-8", rec.Header().Get("Content-Type"))
			assert.Equal(t, "Hello World", rec.Body.String())
		})

		t.Run("protected endpoint requires auth", func(t *testing.T) {
			//t.Parallel() // Subtests can run in parallel

			// Test without auth
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Host = ctx.Config().Config().Core.Domain
			rec := httptest.NewRecorder()

			ctx.Router().ServeHTTP(rec, req)
			assert.Equal(t, http.StatusUnauthorized, rec.Code)

			// Test with valid auth
			pk := ctx.Config().Config().Core.Identity.PrivateKey()
			token, err := jwt.CreateToken(
				pk,
				ctx.Config().Config().Core.Domain,
				"1", // Test user ID
				jwt.PurposeLogin,
				time.Hour,
			)
			require.NoError(t, err)

			req = httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Host = ctx.Config().Config().Core.Domain
			req.Header.Set("Authorization", "Bearer "+token)
			rec = httptest.NewRecorder()

			ctx.Router().ServeHTTP(rec, req)
			assert.Equal(t, http.StatusOK, rec.Code)
		})
	},
		// Pass the API registration option here as a variadic argument
		coreTesting.NewAPIRegistrationOption("test", NewTestAPI),
	)
}

// TestAPI implements a simple API for testing
type TestAPI struct {
	ctx    core.Context
	logger *core.Logger
}

func NewTestAPI() (core.API, []core.ContextBuilderOption, error) {
	api := &TestAPI{}

	opts := []core.ContextBuilderOption{
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			api.ctx = ctx
			api.logger = ctx.NamedLogger("test.api")
			return nil
		}),
	}

	return api, opts, nil
}

func (a *TestAPI) Name() string          { return "test" }
func (a *TestAPI) Subdomain() string     { return "" }
func (a *TestAPI) AuthTokenName() string { return "test-token" }

func (a *TestAPI) OpenAPIInfo() router.APIInfoDefinition {
	return router.APIInfo().
		Title("Test API").
		Description("Test API for unit testing").
		Version("1.0.0")
}

func (a *TestAPI) Config() config.APIConfig {
	return &TestAPIConfig{}
}

func (a *TestAPI) Configure(r router.Router, access core.AccessService) error {
	// Public route
	public := router.DefineRoutes(
		router.NewRoute(http.MethodGet, "/hello", a.helloHandler,
			router.WithSwagger(
				router.WithSummary("Hello endpoint"),
				router.WithDescription("Returns a greeting"),
				router.WithTags("Public"),
				router.WithSuccessResponse(http.StatusOK, "Hello World"),
			),
		),
	)

	// Protected route
	protected := router.DefineRoutes(
		router.NewRoute(http.MethodGet, "/protected", a.protectedHandler,
			router.WithSwagger(
				router.WithSummary("Protected endpoint"),
				router.WithDescription("Requires authentication"),
				router.WithTags("Authenticated"),
				router.WithSuccessResponse(http.StatusOK, "User data"),
				router.WithErrorResponses(
					router.DefineSwaggerErrorResponse(http.StatusUnauthorized, "Unauthorized"),
				),
			),
			router.WithMiddlewares(middleware.AuthMiddleware(
				a.ctx,
				jwt.PurposeLogin,
			)),
		),
	)

	if err := router.RegisterRoutes(r, access, "", public); err != nil {
		return err
	}

	return router.RegisterRoutes(r, access, "", protected)
}

func (a *TestAPI) helloHandler(c echo.Context) error {
	return c.String(http.StatusOK, "Hello World")
}

func (a *TestAPI) protectedHandler(c echo.Context) error {
	userID, err := mcontext.GetUserID(c)
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err)
	}

	if userID == 0 {
		return echo.NewHTTPError(http.StatusUnauthorized)
	}

	return c.JSON(http.StatusOK, map[string]interface{}{
		"userID": userID,
	})
}

// TestAPIConfig implements config.APIConfig
type TestAPIConfig struct{}

func (c *TestAPIConfig) Defaults() map[string]interface{} {
	return map[string]interface{}{}
}

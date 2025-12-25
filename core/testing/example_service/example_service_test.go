package example_service_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/gorm"
)

// TestMain is the entry point for running tests in this package.
// It sets up and tears down the test environment, including the database.
func TestMain(m *testing.M) {
	// Run tests with an in-memory SQLite database enabled by default.
	// Migrations are also enabled by default.
	// We also register the real UserService here so all tests use it.
	os.Exit(coreTesting.WithDBAndOptions(m,
		WithRealUserService(),
	))
}

// WithRealUserService configures the test context to use the real UserService implementation
func WithRealUserService() coreTesting.TestContextBuilderOption {
	return func(ctx coreTesting.TestContext) (coreTesting.TestContext, error) {
		// Create and register the real service
		userSvc, opts, err := service.NewUserService()
		if err != nil {
			return ctx, err
		}

		// Apply any context options from the service
		ctx, err = coreTesting.ProcessCtxOptions(ctx, coreTesting.WrapCoreOptions(opts)...)
		if err != nil {
			return ctx, err
		}

		// Register the real service
		ctx.RegisterService(core.USER_SERVICE, userSvc)
		return ctx, nil
	}
}

// TestUserService demonstrates a complete service test with database integration
func TestUserService(t *testing.T) {
	t.Parallel()

	// SetupTest retrieves the TestContext configured by TestMain.
	// It also registers cleanup for this specific test.
	ctx := coreTesting.SetupTest(t)

	// Boot the environment after the context is set up.
	// This applies the global options (like WithRealUserService) and runs startup funcs.
	err := coreTesting.BootEnvironment(t, ctx)
	require.NoError(t, err, "Failed to boot test environment")

	// Get the real database instance
	db := ctx.DB()

	// Get the real user service
	userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
	require.NotNil(t, userSvc, "User service not found in context after booting environment")

	t.Run("create and get user", func(t *testing.T) {
		//	t.Parallel()

		email := "test@example.com"
		password := "password123"

		// Test create user account using the real service
		createdUser, err := userSvc.CreateAccount(nil, email, password, false)
		require.NoError(t, err)
		assert.Equal(t, email, createdUser.Email)
		assert.NotZero(t, createdUser.ID)

		// Verify user exists in the database
		var userInDB models.User
		result := db.Where("email = ?", email).First(&userInDB)
		require.NoError(t, result.Error)
		assert.Equal(t, createdUser.ID, userInDB.ID)
		assert.Equal(t, createdUser.Email, userInDB.Email)

		// Test check if email exists using the real service
		exists, foundUser, err := userSvc.EmailExists(nil, email)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, createdUser.ID, foundUser.ID)
		assert.Equal(t, createdUser.Email, foundUser.Email)
	})

	t.Run("user not found", func(t *testing.T) {
		//	t.Parallel()

		// Test check if non-existent email exists using the real service
		exists, _, err := userSvc.EmailExists(nil, "nonexistent@example.com")
		assert.NoError(t, err) // EmailExists should return false, nil, nil for not found
		assert.False(t, exists)
	})

	t.Run("update user", func(t *testing.T) {
		//	t.Parallel()

		// Create test user first using the real service
		testUser, err := userSvc.CreateAccount(nil, "update@example.com", "password123", false)
		require.NoError(t, err)

		// Test update account info using the real service
		updatedEmail := "updated@example.com"
		err = userSvc.UpdateAccountInfo(nil, testUser.ID, map[string]any{
			"email": updatedEmail,
		})
		require.NoError(t, err)

		// Verify update in the database
		var updatedUser models.User
		require.NoError(t, db.First(&updatedUser, testUser.ID).Error)
		assert.Equal(t, updatedEmail, updatedUser.Email)
	})

	t.Run("delete user", func(t *testing.T) {
		//	t.Parallel()

		// Create test user first using the real service
		testUser, err := userSvc.CreateAccount(nil, "delete@example.com", "password123", false)
		require.NoError(t, err)

		// Test delete account using the real service
		err = userSvc.DeleteAccount(nil, testUser.ID)
		require.NoError(t, err)

		// Verify deletion in the database
		var deletedUser models.User
		err = db.First(&deletedUser, testUser.ID).Error
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	})

	t.Run("list users", func(t *testing.T) {
		//	t.Parallel()

		// Create test data using the real service
		usersToCreate := []string{"list1@example.com", "list2@example.com", "list3@example.com"}
		for _, email := range usersToCreate {
			_, err := userSvc.CreateAccount(nil, email, "password123", false)
			require.NoError(t, err)
		}

		// Test get accounts pending deletion (using a real method from the interface)
		// This method is not implemented in the mock, but should work with the real service
		// if it queries the database correctly.
		pendingUsers, err := userSvc.GetAccountsPendingDeletion(nil)
		require.NoError(t, err)
		// Since we just created test users, none should be pending deletion
		assert.Len(t, pendingUsers, 0)

		// Note: Testing pagination/listing requires calling the actual List method
		// on the real service, which is not part of the current UserService interface
		// summary provided. If a List method exists on the real service, you would
		// call it here and assert on the results from the database.
		// Example (assuming a List method exists):
		// listedUsers, err := userSvc.List(context.Background(), 2, 1) // limit 2, offset 1
		// require.NoError(t, err)
		// assert.Len(t, listedUsers, 2)
		// assert.Equal(t, "list2@example.com", listedUsers[0].Email) // Assuming default order by creation
	})
}

// BenchmarkUserService demonstrates benchmarking the real UserService with database operations
func BenchmarkUserService(b *testing.B) {
	// SetupTestWithDB creates a test context with DB support.
	// Since TestMain already configured the environment with the real service,
	// this will use the real service and the in-memory DB.
	ctx := coreTesting.SetupTestWithDB(b)
	defer ctx.Teardown() // Ensure cleanup after benchmark

	// Boot the environment after the context is set up.
	// This applies the global options (like WithRealUserService) and runs startup funcs.
	err := coreTesting.BootEnvironment(b, ctx)
	require.NoError(b, err, "Failed to boot test environment")

	// Get the real user service
	userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
	require.NotNil(b, userSvc, "User service not found in context after booting environment")

	// Reset the timer before the loop to exclude setup time
	b.ResetTimer()

	b.Run("create user", func(b *testing.B) {
		// SetupTestWithDB provides a clean DB for each benchmark run (due to t.Cleanup).
		// No need to manually delete users.
		for i := 0; i < b.N; i++ {
			email := fmt.Sprintf("user%d@example.com", i)
			_, err := userSvc.CreateAccount(nil, email, "password123", false)
			if err != nil {
				b.Fatalf("Failed to create user: %v", err)
			}
		}
	})

	b.Run("check email exists", func(b *testing.B) {
		// SetupTestWithDB provides a clean DB for each benchmark run.
		// Create test data within the benchmark setup.
		email := "benchmark@example.com"
		_, err := userSvc.CreateAccount(nil, email, "password123", false)
		if err != nil {
			b.Fatalf("Failed to create test user: %v", err)
		}

		b.ResetTimer() // Reset timer after setup

		for i := 0; i < b.N; i++ {
			_, _, err := userSvc.EmailExists(nil, email)
			if err != nil {
				b.Fatalf("Failed to check email: %v", err)
			}
		}
	})
}

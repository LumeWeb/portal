package example_service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/db/models"
	"go.lumeweb.com/portal/service"
	"gorm.io/gorm"
)

// TestUserService_CreateAndGet tests creating and retrieving a user
func TestUserService_CreateAndGet(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userSvc)

		email := "test@example.com"
		password := "password123"

		createdUser, err := userSvc.CreateAccount(context.Background(), email, password, false)
		require.NoError(t, err)
		assert.Equal(t, email, createdUser.Email)
		assert.NotZero(t, createdUser.ID)

		var userInDB models.User
		result := ctx.DB().Where("email = ?", email).First(&userInDB)
		require.NoError(t, result.Error)
		assert.Equal(t, createdUser.ID, userInDB.ID)

		exists, foundUser, err := userSvc.EmailExists(context.Background(), email)
		require.NoError(t, err)
		assert.True(t, exists)
		assert.Equal(t, createdUser.ID, foundUser.ID)
	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
	)
}

// TestUserService_NotFound tests email exists for non-existent user
func TestUserService_NotFound(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userSvc)

		exists, _, err := userSvc.EmailExists(context.Background(), "nonexistent@example.com")
		assert.NoError(t, err)
		assert.False(t, exists)
	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
	)
}

// TestUserService_Update tests updating a user
func TestUserService_Update(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userSvc)

		testUser, err := userSvc.CreateAccount(context.Background(), "update@example.com", "password123", false)
		require.NoError(t, err)

		updatedEmail := "updated@example.com"
		err = userSvc.UpdateAccountInfo(context.Background(), testUser.ID, map[string]any{
			"email": updatedEmail,
		})
		require.NoError(t, err)

		var updatedUser models.User
		require.NoError(t, ctx.DB().First(&updatedUser, testUser.ID).Error)
		assert.Equal(t, updatedEmail, updatedUser.Email)
	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
	)
}

// TestUserService_Delete tests deleting a user
func TestUserService_Delete(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userSvc)

		testUser, err := userSvc.CreateAccount(context.Background(), "delete@example.com", "password123", false)
		require.NoError(t, err)

		err = userSvc.DeleteAccount(context.Background(), testUser.ID)
		require.NoError(t, err)

		var deletedUser models.User
		err = ctx.DB().First(&deletedUser, testUser.ID).Error
		assert.ErrorIs(t, err, gorm.ErrRecordNotFound)
	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
	)
}

// TestUserService_List tests listing users pending deletion
func TestUserService_List(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
		require.NotNil(tb, userSvc)

		usersToCreate := []string{"list1@example.com", "list2@example.com", "list3@example.com"}
		for _, email := range usersToCreate {
			_, err := userSvc.CreateAccount(context.Background(), email, "password123", false)
			require.NoError(t, err)
		}

		pendingUsers, err := userSvc.GetAccountsPendingDeletion(context.Background())
		require.NoError(t, err)
		assert.Len(t, pendingUsers, 0)
	},
		coreTesting.WithServiceFactory(core.USER_SERVICE, service.NewUserService),
	)
}

// BenchmarkUserService benchmarks the real UserService with database operations
func BenchmarkUserService(b *testing.B) {
	// For benchmarks, we use SetupTestWithDB directly since RunTestCaseWithDB
	// expects a function with TB interface which doesn't have benchmark methods
	ctx, err := coreTesting.SetupTestWithDB(b)
	require.NoError(b, err)

	realUserSvc, opts, err := service.NewUserService()
	require.NoError(b, err)

	ctx, err = coreTesting.ProcessCtxOptions(ctx, coreTesting.WrapCoreOptions(opts)...)
	require.NoError(b, err)

	err = coreTesting.BootEnvironment(b, ctx)
	require.NoError(b, err)

	userSvc := core.GetService[core.UserService](ctx, core.USER_SERVICE)
	require.NotNil(b, userSvc)
	_ = realUserSvc // unused but kept for clarity

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		_, err := userSvc.CreateAccount(context.Background(), email, "password123", false)
		if err != nil {
			b.Fatalf("Failed to create user: %v", err)
		}
	}
}
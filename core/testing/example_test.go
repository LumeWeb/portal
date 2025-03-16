package testing_test

import (
	"github.com/DATA-DOG/go-sqlmock"
	"testing"

	"go.lumeweb.com/portal/core"
	coretesting "go.lumeweb.com/portal/core/testing"
	testdb "go.lumeweb.com/portal/core/testing/db"
)

func TestExample(t *testing.T) {
	// Reset all global state before the test
	coretesting.ResetAllState()

	// Create a mock auth service
	mockAuth := coretesting.NewMockAuthService()

	// Create a test context with the mock service
	ctx := coretesting.NewTestContext(t,
		coretesting.WithMockService(core.AUTH_SERVICE, mockAuth),
		coretesting.WithConfigValue("core.domain", "example.com"),
	)
	defer ctx.Teardown() // Clean up resources when test completes

	// Register the test event
	testEvent := &core.Event{}
	core.RegisterEvent("user.created", testEvent)

	// Create an event recorder
	recorder := coretesting.NewEventRecorder()
	recorder.Listen(ctx, "user.created", "user.login")

	// Fire the event
	err := ctx.Event().FireEvent(testEvent)
	if err != nil {
		t.Fatalf("Failed to fire event: %v", err)
	}

	// Verify events were fired
	if !recorder.HasEvent("user.created") {
		t.Error("Expected user.created event to be fired")
	}
}

func TestDynamicServiceRegistration(t *testing.T) {
	// Create a test context
	ctx := coretesting.NewTestContext(t)
	defer ctx.Teardown()

	// Dynamically register a service during the test
	mockUserService := coretesting.NewMockService(core.USER_SERVICE)
	ctx.RegisterService(core.USER_SERVICE, mockUserService)

	// Verify the service was registered
	if ctx.Service(core.USER_SERVICE) == nil {
		t.Error("Expected user service to be registered")
	}
}

func TestDatabaseOperations(t *testing.T) {
	// Create a context with a mock DB
	ctx, mock := testdb.SetupSQLMock(t)
	defer ctx.Teardown()

	// Set up expectations
	mock.ExpectQuery("SELECT").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(1))

	// Use the DB in your test
	db := ctx.DB()
	var result struct{ ID int }
	db.Raw("SELECT id FROM users LIMIT 1").Scan(&result)

	// Verify all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

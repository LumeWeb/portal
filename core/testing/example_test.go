package testing_test

import (
	"testing"

	"go.lumeweb.com/portal/core"
	coretesting "go.lumeweb.com/portal/core/testing"
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

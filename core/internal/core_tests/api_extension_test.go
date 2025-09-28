package core_tests

import (
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/core/testing/mocks"
	"testing"

	"github.com/stretchr/testify/assert"
	. "go.lumeweb.com/portal/core"
)

func TestRegisterAPIExtension(t *testing.T) {
	ResetState() // Ensure a clean state

	mockExt1 := mocks.NewMockAPIExtension(t)
	mockExt1.On("TargetAPI").Return("api1").Once()

	RegisterAPIExtension(mockExt1)

	extensions := GetAPIExtensions("api1")
	assert.Len(t, extensions, 1)
	assert.Equal(t, mockExt1, extensions[0])

	mockExt1.AssertExpectations(t)
}

func TestGetAPIExtensions(t *testing.T) {
	ResetState() // Ensure a clean state

	mockExt1 := mocks.NewMockAPIExtension(t)
	mockExt1.On("TargetAPI").Return("apiA").Once()
	mockExt2 := mocks.NewMockAPIExtension(t)
	mockExt2.On("TargetAPI").Return("apiB").Once()

	RegisterAPIExtension(mockExt1)
	RegisterAPIExtension(mockExt2)

	extensionsA := GetAPIExtensions("apiA")
	assert.Len(t, extensionsA, 1)
	assert.Equal(t, mockExt1, extensionsA[0])

	extensionsB := GetAPIExtensions("apiB")
	assert.Len(t, extensionsB, 1)
	assert.Equal(t, mockExt2, extensionsB[0])

	mockExt1.AssertExpectations(t)
	mockExt2.AssertExpectations(t)
}

func TestGetAPIExtensions_NotFound(t *testing.T) {
	ResetState() // Ensure a clean state

	extensions := GetAPIExtensions("nonexistent-api")
	assert.Nil(t, extensions)
}

func TestGetAPIExtensions_MultipleExtensions(t *testing.T) {
	ResetState() // Ensure a clean state

	mockExt1 := mocks.NewMockAPIExtension(t)
	mockExt1.On("TargetAPI").Return("apiC").Once()
	mockExt2 := mocks.NewMockAPIExtension(t)
	mockExt2.On("TargetAPI").Return("apiC").Once()
	mockExt3 := mocks.NewMockAPIExtension(t)
	mockExt3.On("TargetAPI").Return("apiD").Once()

	RegisterAPIExtension(mockExt1)
	RegisterAPIExtension(mockExt2)
	RegisterAPIExtension(mockExt3)

	extensionsC := GetAPIExtensions("apiC")
	assert.Len(t, extensionsC, 2)
	// Order is not guaranteed, so check for presence
	assert.Contains(t, extensionsC, mockExt1)
	assert.Contains(t, extensionsC, mockExt2)

	extensionsD := GetAPIExtensions("apiD")
	assert.Len(t, extensionsD, 1)
	assert.Equal(t, mockExt3, extensionsD[0])

	mockExt1.AssertExpectations(t)
	mockExt2.AssertExpectations(t)
	mockExt3.AssertExpectations(t)
}

func TestResetEvents(t *testing.T) {
	ctx, err := coreTesting.NewTestContext(t)
	assert.NoError(t, err)
	recorder := coreTesting.NewEventRecorder()

	// Listen to test events
	recorder.Listen(ctx, "test-event-1", "test-event-2")

	// Fire some events
	err = ctx.Fire("test-event-1", nil)
	assert.NoError(t, err)
	err = ctx.Fire("test-event-2", nil)
	assert.NoError(t, err)

	// Verify events were recorded
	assert.True(t, recorder.HasEvent("test-event-1"))
	assert.True(t, recorder.HasEvent("test-event-2"))

	// Reset events
	ctx.ResetEvents()
	recorder.Reset()

	// Verify recorder was reset
	assert.False(t, recorder.HasEvent("test-event-1"))
	assert.False(t, recorder.HasEvent("test-event-2"))
}

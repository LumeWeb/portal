package core_tests

import (
	"go.lumeweb.com/portal/core/testing/mocks"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	. "go.lumeweb.com/portal/core"
)

func TestRegisterAPI(t *testing.T) {
	ResetState() // Ensure a clean state

	mockAPI := mocks.NewMockAPI(t)
	RegisterAPI("test-api", mockAPI)

	retrievedAPI := GetAPI("test-api")
	assert.NotNil(t, retrievedAPI)
}

func TestRegisterAPI_DuplicateID(t *testing.T) {
	ResetState() // Ensure a clean state

	mockAPI1 := mocks.NewMockAPI(t)
	mockAPI2 := mocks.NewMockAPI(t)

	RegisterAPI("test-api", mockAPI1)

	assert.Panics(t, func() {
		RegisterAPI("test-api", mockAPI2)
	}, "Registering duplicate API ID should panic")

}

func TestGetAPI(t *testing.T) {
	ResetState() // Ensure a clean state

	mockAPI := mocks.NewMockAPI(t)
	RegisterAPI("test-api", mockAPI)

	retrievedAPI := GetAPI("test-api")
	assert.NotNil(t, retrievedAPI)
}

func TestGetAPI_NotFound(t *testing.T) {
	ResetState() // Ensure a clean state

	retrievedAPI := GetAPI("non-existent-api")
	assert.Nil(t, retrievedAPI)
}

func TestAPIExists(t *testing.T) {
	ResetState() // Ensure a clean state

	mockAPI := mocks.NewMockAPI(t)

	RegisterAPI("test-api", mockAPI)

	assert.True(t, APIExists("test-api"))
	assert.False(t, APIExists("non-existent-api"))

}

func TestGetAPIs(t *testing.T) {
	ResetState() // Ensure a clean state

	mockAPI1 := mocks.NewMockAPI(t)
	mockAPI2 := mocks.NewMockAPI(t)

	RegisterAPI("api-b", mockAPI1)
	RegisterAPI("api-a", mockAPI2)

	apisMap := GetAPIs()
	assert.Len(t, apisMap, 2)
	assert.Contains(t, apisMap, "api-a")
	assert.Contains(t, apisMap, "api-b")
}

func TestGetAPIList(t *testing.T) {
	ResetState() // Ensure a clean state

	mockAPI1 := mocks.NewMockAPI(t)
	mockAPI2 := mocks.NewMockAPI(t)

	RegisterAPI("api-b", mockAPI1)
	RegisterAPI("api-a", mockAPI2)

	apiList := GetAPIList()
	require.Len(t, apiList, 2)
}

func TestResetAPIs(t *testing.T) {
	// Register some test APIs
	mockAPI1 := mocks.NewMockAPI(t)
	mockAPI2 := mocks.NewMockAPI(t)
	RegisterAPI("test-api-1", mockAPI1)
	RegisterAPI("test-api-2", mockAPI2)

	// Check APIs exist
	assert.True(t, APIExists("test-api-1"))
	assert.True(t, APIExists("test-api-2"))

	// Reset APIs
	ResetAPIs()

	// Check APIs no longer exist
	assert.False(t, APIExists("test-api-1"))
	assert.False(t, APIExists("test-api-2"))
	assert.Empty(t, GetAPIs())
	assert.Empty(t, GetAPIList())
}

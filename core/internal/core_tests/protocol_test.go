package core_tests

import (
	"go.lumeweb.com/portal/core"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.lumeweb.com/portal/core/testing/mocks"
)

func TestRegisterProtocol(t *testing.T) {
	core.ResetState()
	mockProtocol := mocks.NewMockProtocol(t)

	core.RegisterProtocol("test-protocol", mockProtocol)

	protocols := core.GetProtocols()
	assert.Len(t, protocols, 1)
	assert.Contains(t, protocols, "test-protocol")
	assert.Equal(t, mockProtocol, protocols["test-protocol"])
}

func TestRegisterProtocol_DuplicateID(t *testing.T) {
	core.ResetState()
	mockProtocol1 := mocks.NewMockProtocol(t)
	mockProtocol2 := mocks.NewMockProtocol(t)

	core.RegisterProtocol("test-protocol", mockProtocol1)

	assert.PanicsWithValue(t, "protocol already registered: test-protocol", func() {
		core.RegisterProtocol("test-protocol", mockProtocol2)
	})
}

func TestGetProtocol(t *testing.T) {
	core.ResetState()
	mockProtocol := mocks.NewMockProtocol(t)

	core.RegisterProtocol("test-protocol", mockProtocol)

	retrievedProtocol := core.GetProtocol("test-protocol")
	assert.Equal(t, mockProtocol, retrievedProtocol)
}

func TestGetProtocol_NotFound(t *testing.T) {
	core.ResetState()
	retrievedProtocol := core.GetProtocol("non-existent-protocol")
	assert.Nil(t, retrievedProtocol)
}

func TestProtocolExists(t *testing.T) {
	core.ResetState()
	mockProtocol := mocks.NewMockProtocol(t)

	core.RegisterProtocol("test-protocol", mockProtocol)

	assert.True(t, core.ProtocolExists("test-protocol"))
	assert.False(t, core.ProtocolExists("non-existent-protocol"))
}

func TestGetProtocols(t *testing.T) {
	core.ResetState()
	mockProtocol1 := mocks.NewMockProtocol(t)
	mockProtocol2 := mocks.NewMockProtocol(t)

	core.RegisterProtocol("protocol-a", mockProtocol1)
	core.RegisterProtocol("protocol-b", mockProtocol2)

	protocols := core.GetProtocols()
	assert.Len(t, protocols, 2)
	assert.Contains(t, protocols, "protocol-a")
	assert.Contains(t, protocols, "protocol-b")
	assert.Equal(t, mockProtocol1, protocols["protocol-a"])
	assert.Equal(t, mockProtocol2, protocols["protocol-b"])
}

func TestGetProtocolList(t *testing.T) {
	core.ResetState()
	mockProtocol1 := mocks.NewMockProtocol(t)
	mockProtocol2 := mocks.NewMockProtocol(t)

	core.RegisterProtocol("protocol-b", mockProtocol1)
	core.RegisterProtocol("protocol-a", mockProtocol2)

	protocolList := core.GetProtocolList()
	assert.Len(t, protocolList, 2)
	// Should be sorted by name
	assert.Equal(t, mockProtocol2, protocolList[0]) // protocol-a
	assert.Equal(t, mockProtocol1, protocolList[1]) // protocol-b
}

func TestProtocolHasDataRequestHandler(t *testing.T) {
	core.ResetState()
	// Use the new composite mock
	mockProtocolWithHandler := mocks.NewMockTestingProtocolRequestDataHandler(t)
	mockProtocolWithoutHandler := mocks.NewMockProtocol(t)

	core.RegisterProtocol("protocol-with-handler", mockProtocolWithHandler)
	core.RegisterProtocol("protocol-without-handler", mockProtocolWithoutHandler)

	assert.True(t, core.ProtocolHasDataRequestHandler("protocol-with-handler"))
	assert.False(t, core.ProtocolHasDataRequestHandler("protocol-without-handler"))
	assert.False(t, core.ProtocolHasDataRequestHandler("non-existent-protocol"))
}

func TestGetProtocolDataRequestHandler(t *testing.T) {
	core.ResetState()
	// Use the new composite mock
	mockProtocolWithHandler := mocks.NewMockTestingProtocolRequestDataHandler(t)
	mockProtocolWithoutHandler := mocks.NewMockProtocol(t)

	core.RegisterProtocol("protocol-with-handler", mockProtocolWithHandler)
	core.RegisterProtocol("protocol-without-handler", mockProtocolWithoutHandler)

	handler := core.GetProtocolDataRequestHandler("protocol-with-handler")
	assert.Equal(t, mockProtocolWithHandler, handler)

	assert.PanicsWithValue(t, "protocol not found: non-existent-protocol", func() {
		core.GetProtocolDataRequestHandler("non-existent-protocol")
	})

	assert.PanicsWithValue(t, "protocol does not have a request handler: *mocks.MockProtocol", func() {
		core.GetProtocolDataRequestHandler("protocol-without-handler")
	})
}

func TestProtocolHasPinHandler(t *testing.T) {
	core.ResetState()
	// Use the new composite mock
	mockProtocolWithHandler := mocks.NewMockTestingProtocolPinHandler(t)
	mockProtocolWithoutHandler := mocks.NewMockProtocol(t)

	core.RegisterProtocol("protocol-with-handler", mockProtocolWithHandler)
	core.RegisterProtocol("protocol-without-handler", mockProtocolWithoutHandler)

	assert.True(t, core.ProtocolHasPinHandler("protocol-with-handler"))
	assert.False(t, core.ProtocolHasPinHandler("protocol-without-handler"))
	assert.False(t, core.ProtocolHasPinHandler("non-existent-protocol"))
}

func TestGetProtocolPinHandler(t *testing.T) {
	core.ResetState()
	// Use the new composite mock
	mockProtocolWithHandler := mocks.NewMockTestingProtocolPinHandler(t)
	mockProtocolWithoutHandler := mocks.NewMockProtocol(t)

	core.RegisterProtocol("protocol-with-handler", mockProtocolWithHandler)
	core.RegisterProtocol("protocol-without-handler", mockProtocolWithoutHandler)

	handler := core.GetProtocolPinHandler("protocol-with-handler")
	assert.Equal(t, mockProtocolWithHandler, handler)

	assert.PanicsWithValue(t, "protocol not found: non-existent-protocol", func() {
		core.GetProtocolPinHandler("non-existent-protocol")
	})

	assert.PanicsWithValue(t, "protocol does not have a data pin handler: *mocks.MockProtocol", func() {
		core.GetProtocolPinHandler("protocol-without-handler")
	})
}

func TestResetProtocols(t *testing.T) {
	// Register some test protocols
	mockProtocol1 := mocks.NewMockProtocol(t)
	mockProtocol2 := mocks.NewMockProtocol(t)
	core.RegisterProtocol("test-protocol-1", mockProtocol1)
	core.RegisterProtocol("test-protocol-2", mockProtocol2)

	// Check protocols exist
	assert.True(t, core.ProtocolExists("test-protocol-1"))
	assert.True(t, core.ProtocolExists("test-protocol-2"))

	// Reset protocols
	core.ResetProtocols()

	// Check protocols no longer exist
	assert.False(t, core.ProtocolExists("test-protocol-1"))
	assert.False(t, core.ProtocolExists("test-protocol-2"))
	assert.Empty(t, core.GetProtocols())
	assert.Empty(t, core.GetProtocolList())
}

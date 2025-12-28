package testing

import (
	router "go.lumeweb.com/portal-router"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
)

var _ core.APIExtension = (*MockAPIExtension)(nil)

// MockAPIExtension implements core.APIExtension for testing
type MockAPIExtension struct {
	*mocks.MockAPIExtension
	targetAPIValue string
	configureFunc  func(router.Router, core.AccessService) error
}

func (m *MockAPIExtension) TargetAPI() string {
	return m.targetAPIValue
}

func (m *MockAPIExtension) Configure(router router.Router, accessSvc core.AccessService) error {
	if m.configureFunc != nil {
		return m.configureFunc(router, accessSvc)
	}
	return m.MockAPIExtension.Configure(router, accessSvc)
}

// NewMockAPIExtension creates a new mock API extension
func NewMockAPIExtension(t TB, targetAPI string) *MockAPIExtension {
	mockAPIExtension := &MockAPIExtension{
		MockAPIExtension: mocks.NewMockAPIExtension(t),
		targetAPIValue:   targetAPI,
	}

	// Setup default expectations
	mockAPIExtension.MockAPIExtension.EXPECT().TargetAPI().Return(targetAPI).Maybe()
	mockAPIExtension.MockAPIExtension.EXPECT().Configure(mock.Anything, mock.Anything).Return(nil).Maybe()

	return mockAPIExtension
}

// WithConfigure sets a custom Configure function for the MockAPIExtension
func (m *MockAPIExtension) WithConfigure(f func(router.Router, core.AccessService) error) *MockAPIExtension {
	m.configureFunc = f
	return m
}

// Ensure MockAPIExtension implements core.APIExtension
var _ core.APIExtension = (*MockAPIExtension)(nil)

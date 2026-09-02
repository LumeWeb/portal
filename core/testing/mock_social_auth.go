package testing

import (
	"github.com/stretchr/testify/mock"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/core/testing/mocks"
	"go.lumeweb.com/portal/db/models"
)

// MockSocialAuthService provides high-level helper functions that embed
// mocks.MockSocialAuthService. It automatically sets up mock expectations and
// provides convenient methods for common social auth operations.
type MockSocialAuthService struct {
	*mocks.MockSocialAuthService
	ctx TestContext
}

// NewMockSocialAuthService creates a new MockSocialAuthService that embeds the
// generated social auth mock.
func NewMockSocialAuthService(ctx TestContext) *MockSocialAuthService {
	return &MockSocialAuthService{
		MockSocialAuthService: mocks.NewMockSocialAuthService(ctx.T()),
		ctx:                   ctx,
	}
}

// RegisterLinkedLogin pre-registers a provider identity as already linked, so a
// subsequent LoginOrLink for that (provider, providerUserID) returns the given
// user as an existing link (Created=false, Linked=false).
func (m *MockSocialAuthService) RegisterLinkedLogin(provider, providerUserID string, user *models.User) {
	m.EXPECT().LoginOrLink(mock.Anything, provider, providerUserID, mock.Anything, mock.Anything).
		Return(&core.SocialAuthResult{User: user}, nil)
}

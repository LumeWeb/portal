package core

import "context"

const (
	ACCESS_SERVICE = "access"

	ACCESS_ADMIN_ROLE = "admin"
	ACCESS_USER_ROLE  = "user"
)

// AccessService interface defines the simplified methods for managing access control
type AccessService interface {
	// RegisterRoute adds a new route with its associated role and permissions
	RegisterRoute(ctx context.Context, subdomain, path, method, role string) error

	// RegisterRole adds a new role with its associated permissions
	AssignRoleToUser(ctx context.Context, userId uint, role string) error

	// CheckAccess checks if a given role has access to a specific route
	CheckAccess(ctx context.Context, userId uint, fqdn, path, method string) (bool, error)

	// ExportUserPolicy returns the policy for a specific user
	ExportUserPolicy(ctx context.Context, userId uint) ([]*AccessPolicy, error)

	// ExportModel returns the model for the access service
	ExportModel(ctx context.Context) *AccessModel

	Service
}

type AccessPolicy struct {
	Subject string `json:"sub"`
	Domain  string `json:"dom"`
	Object  string `json:"obj"`
	Action  string `json:"act"`
}

type AccessModelDef struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type AccessModel struct {
	RequestDefinition AccessModelDef `json:"request_definition"`
	PolicyDefinition  AccessModelDef `json:"policy_definition"`
	RoleDefinition    AccessModelDef `json:"role_definition"`
	PolicyEffect      AccessModelDef `json:"policy_effect"`
	Matchers          AccessModelDef `json:"matchers"`
}

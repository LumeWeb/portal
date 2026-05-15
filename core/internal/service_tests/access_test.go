package service_tests

import (
	"fmt"
	"strconv"
	"testing"

	"github.com/casbin/casbin/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.lumeweb.com/portal/core"
	coreTesting "go.lumeweb.com/portal/core/testing"
	"go.lumeweb.com/portal/service"
)

func TestAccessServiceDefault_RegisterRoute(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		accessService := core.GetService[*service.AccessServiceDefault](ctx, core.ACCESS_SERVICE)
		require.NotNil(tb, accessService)

		subdomain := "test"
		path := "/testpath"
		method := "GET"
		role := "testrole"

		err := accessService.RegisterRoute(nil, subdomain, path, method, role)
		assert.NoError(tb, err)

		fqdn := fmt.Sprintf("%s.%s", subdomain, ctx.Config().Config().Core.Domain)
		enforce, err := accessService.GetEnforcer().Enforce(role, fqdn, path, method)
		assert.NoError(tb, err)
		assert.True(tb, enforce)
	}, coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService))
}

func TestAccessServiceDefault_AssignRoleToUser(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		accessService := core.GetService[*service.AccessServiceDefault](ctx, core.ACCESS_SERVICE)
		require.NotNil(tb, accessService)

		userID := uint(123)
		role := "testrole"

		err := accessService.AssignRoleToUser(nil, userID, role)
		assert.NoError(tb, err)

		userIDStr := strconv.FormatUint(uint64(userID), 10)
		roles, err := accessService.GetEnforcer().GetRolesForUser(userIDStr)
		assert.NoError(tb, err)
		assert.Contains(tb, roles, role)
	}, coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService))
}

func TestAccessServiceDefault_CheckAccess(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		accessService := core.GetService[*service.AccessServiceDefault](ctx, core.ACCESS_SERVICE)
		require.NotNil(tb, accessService)

		userID := uint(123)
		fqdn := "test.example.com"
		path := "/testpath"
		method := "GET"

		// Add policy to allow access
		_, err := accessService.GetEnforcer().AddPolicy("testrole", fqdn, path, method)
		assert.NoError(tb, err)

		// Assign role to user
		err = accessService.AssignRoleToUser(nil, userID, "testrole")
		assert.NoError(tb, err)

		// Check access
		access, err := accessService.CheckAccess(nil, userID, fqdn, path, method)
		assert.NoError(tb, err)
		assert.True(tb, access)

		// Check access with incorrect parameters
		access, err = accessService.CheckAccess(nil, userID, fqdn, "/wrongpath", method)
		assert.NoError(tb, err)
		assert.False(tb, access)
	}, coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService))
}

func TestAccessServiceDefault_ExportUserPolicy(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		accessService := core.GetService[*service.AccessServiceDefault](ctx, core.ACCESS_SERVICE)
		require.NotNil(tb, accessService)

		userID := uint(123)
		fqdn := "test.example.com"
		path := "/testpath"
		method := "GET"
		role := "testrole"

		// Add policy
		_, err := accessService.GetEnforcer().AddPolicy(role, fqdn, path, method)
		assert.NoError(tb, err)

		// Assign role to user
		err = accessService.AssignRoleToUser(nil, userID, role)
		assert.NoError(tb, err)

		// Export user policy
		policies, err := accessService.ExportUserPolicy(nil, userID)
		assert.NoError(tb, err)
		assert.NotEmpty(tb, policies)

		found := false
		for _, policy := range policies {
			if policy.Subject == role && policy.Domain == fqdn && policy.Object == path && policy.Action == method {
				found = true
				break
			}
		}
		assert.True(tb, found)

		// Verify that role-based policies are returned (user ID may not appear as a subject
		// unless there are policies directly assigned to the user ID)
	}, coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService))
}

func TestAccessServiceDefault_init(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		accessService := core.GetService[*service.AccessServiceDefault](ctx, core.ACCESS_SERVICE)
		require.NotNil(tb, accessService)

		err := accessService.IInit()
		assert.NoError(tb, err)

		assert.NotNil(tb, accessService.GetEnforcer())
		assert.IsType(tb, &casbin.Enforcer{}, accessService.GetEnforcer())
	}, coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService))
}

func TestAccessServiceDefault_ExportModel(t *testing.T) {
	coreTesting.RunTestCaseWithDB(t, func(tb coreTesting.TB, ctx coreTesting.TestContext) {
		accessService := core.GetService[*service.AccessServiceDefault](ctx, core.ACCESS_SERVICE)
		require.NotNil(tb, accessService)

		model := accessService.ExportModel(nil)
		assert.NotNil(tb, model)
		assert.NotEmpty(tb, model.RequestDefinition.Value)
		assert.NotEmpty(tb, model.PolicyDefinition.Value)
		assert.NotEmpty(tb, model.RoleDefinition.Value)
		assert.NotEmpty(tb, model.PolicyEffect.Value)
		assert.NotEmpty(tb, model.Matchers.Value)
	}, coreTesting.WithServiceFactory(core.ACCESS_SERVICE, service.NewAccessService))
}

package service

import (
	"fmt"
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	_ "github.com/casbin/casbin/v2/rbac/default-role-manager"
	"github.com/casbin/gorm-adapter/v3"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"reflect"
	"strconv"
)

var _ core.AccessService = (*AccessServiceDefault)(nil)

type AccessServiceDefault struct {
	ctx      core.Context
	enforcer *casbin.Enforcer
}

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.ACCESS_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewAccessService()
		},
	})
}

func NewAccessService() (*AccessServiceDefault, []core.ContextBuilderOption, error) {
	service := &AccessServiceDefault{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			service.ctx = ctx

			return service.init()
		}),
	)

	return service, opts, nil
}

func (a *AccessServiceDefault) ID() string {
	return core.ACCESS_SERVICE
}

func (a *AccessServiceDefault) RegisterRoute(subdomain, path, method, role string) error {
	fqdn := fmt.Sprintf("%s.%s", subdomain, a.ctx.Config().Config().Core.Domain)
	_, err := a.enforcer.AddPolicy(role, fqdn, path, method)
	return err
}

func (a *AccessServiceDefault) AssignRoleToUser(userId uint, role string) error {
	userIdStr := strconv.FormatUint(uint64(userId), 10)
	_, err := a.enforcer.AddRoleForUser(userIdStr, role)

	return err
}

func (a *AccessServiceDefault) CheckAccess(userId uint, fqdn, path, method string) (bool, error) {
	return a.enforcer.Enforce(userId, fqdn, path, method)
}

func (a *AccessServiceDefault) ExportUserPolicy(userId uint) ([]*core.AccessPolicy, error) {
	userIdStr := strconv.FormatUint(uint64(userId), 10)
	// Get all roles for the user
	roles, err := a.enforcer.GetRolesForUser(userIdStr)
	if err != nil {
		return nil, err
	}

	// Add the user ID itself to the roles slice
	roles = append(roles, userIdStr)

	var policies []*core.AccessPolicy

	// For each role (including the user ID)
	for _, role := range roles {
		// Get policies for this role
		rolePolicies, err := a.enforcer.GetFilteredPolicy(0, role)
		if err != nil {
			return nil, err
		}

		// Format each policy
		for _, policy := range rolePolicies {
			if len(policy) >= 4 {
				policyStruct := &core.AccessPolicy{
					Subject: policy[0],
					Domain:  policy[1],
					Object:  policy[2],
					Action:  policy[3],
				}
				policies = append(policies, policyStruct)
			}
		}
	}

	return policies, nil
}

func (a *AccessServiceDefault) init() error {
	m := model.NewModel()

	// Request definition
	m.AddDef("r", "r", "sub, dom, obj, act")

	// Policy definition
	m.AddDef("p", "p", "sub, dom, obj, act")

	// Role definition
	m.AddDef("g", "g", "_, _, _")

	// Policy effect
	m.AddDef("e", "e", "some(where (p.eft == allow))")

	// Matchers
	m.AddDef("m", "m", "g(r.sub, p.sub, r.dom) && r.dom == p.dom && keyMatch5(r.obj, p.obj) && r.act == p.act")

	// Load the model
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return err
	}

	a.enforcer = enforcer

	db := a.ctx.DB()

	// Load policies from database
	gormadapter.TurnOffAutoMigrate(db)
	tbl := models.AccessRule{}
	tableName := db.NamingStrategy.TableName(reflect.TypeOf(tbl).Name())
	adapter, _ := gormadapter.NewAdapterByDBWithCustomTable(db, &tbl, tableName)

	return a.enforcer.InitWithModelAndAdapter(m, adapter)
}

func (a *AccessServiceDefault) ExportModel() *core.AccessModel {
	m := a.enforcer.GetModel()
	accessModel := &core.AccessModel{}

	for sec, assertion := range m {
		for key, ast := range assertion {
			def := core.AccessModelDef{
				Key:   key,
				Value: ast.Value,
			}

			switch sec {
			case "r":
				accessModel.RequestDefinition = def
			case "p":
				accessModel.PolicyDefinition = def
			case "g":
				accessModel.RoleDefinition = def
			case "e":
				accessModel.PolicyEffect = def
			case "m":
				accessModel.Matchers = def
			}
		}
	}

	return accessModel
}

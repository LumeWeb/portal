package service

import (
	"context"
	"fmt"
	"reflect"
	"strconv"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	_ "github.com/casbin/casbin/v3/rbac/default-role-manager"
	gormadapter "github.com/casbin/gorm-adapter/v3"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	access "go.lumeweb.com/portal/service/internal/access"
)

var _ core.AccessService = (*AccessServiceDefault)(nil)

type AccessServiceDefault struct {
	*core.BaseComponent

	enforcer *casbin.Enforcer
}

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.ACCESS_SERVICE,
		Factory: NewAccessService,
		Metrics: access.GetCollectors(),
	})
}

func NewAccessService() (core.Service, []core.ContextBuilderOption, error) {
	service := &AccessServiceDefault{}
	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(_ core.Context) error {
			return service.IInit()
		}),
	)

	return service, opts, nil
}

func (a *AccessServiceDefault) ID() string {
	return core.ACCESS_SERVICE
}

func (a *AccessServiceDefault) RegisterRoute(ctx context.Context, subdomain, path, method, role string) error {
	ctx, span := core.TraceMethod(ctx, "AccessServiceDefault.RegisterRoute")
	defer span.End()

	return core.MetricTrack(
		access.AccessDuration.WithLabelValues(access.LabelOpRegisterRoute),
		access.AccessFailed.WithLabelValues(access.LabelOpRegisterRoute),
		func() error {
			fqdn := fmt.Sprintf("%s.%s", subdomain, a.Context().Config().Config().Core.Domain)
			_, err := a.enforcer.AddPolicy(role, fqdn, path, method)
			if err == nil {
				access.RoutesRegistered.WithLabelValues(access.LabelOpRegisterRoute).Inc()
			}
			return err
		},
	)
}

func (a *AccessServiceDefault) AssignRoleToUser(ctx context.Context, userId uint, role string) error {
	ctx, span := core.TraceMethod(ctx, "AccessServiceDefault.AssignRoleToUser")
	defer span.End()

	return core.MetricTrack(
		access.AccessDuration.WithLabelValues(access.LabelOpAssignRole),
		access.AccessFailed.WithLabelValues(access.LabelOpAssignRole),
		func() error {
			userIdStr := strconv.FormatUint(uint64(userId), 10)
			_, err := a.enforcer.AddRoleForUser(userIdStr, role)
			if err == nil {
				access.RolesAssigned.WithLabelValues(access.LabelOpAssignRole).Inc()
			}
			return err
		},
	)
}

func (a *AccessServiceDefault) CheckAccess(ctx context.Context, userId uint, fqdn, path, method string) (bool, error) {
	ctx, span := core.TraceMethod(ctx, "AccessServiceDefault.CheckAccess")
	defer span.End()

	result, err := core.MetricTrackResult(
		access.AccessDuration.WithLabelValues(access.LabelOpCheckAccess),
		access.AccessFailed.WithLabelValues(access.LabelOpCheckAccess),
		func() (bool, error) {
			return a.enforcer.Enforce(strconv.FormatUint(uint64(userId), 10), fqdn, path, method)
		},
	)

	if err == nil {
		access.AccessChecked.WithLabelValues(access.LabelOpCheckAccess).Inc()
	}
	return result, err
}

func (a *AccessServiceDefault) ExportUserPolicy(ctx context.Context, userId uint) ([]*core.AccessPolicy, error) {
	ctx, span := core.TraceMethod(ctx, "AccessServiceDefault.ExportUserPolicy")
	defer span.End()

	result, err := core.MetricTrackResult(
		access.AccessDuration.WithLabelValues(access.LabelOpExportPolicy),
		access.AccessFailed.WithLabelValues(access.LabelOpExportPolicy),
		func() ([]*core.AccessPolicy, error) {
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
		},
	)

	if err == nil {
		access.PolicyExported.WithLabelValues(access.LabelOpExportPolicy).Inc()
	}
	return result, err
}

func (a *AccessServiceDefault) IInit() error {
	m := model.NewModel()

	// Request definition
	m.AddDef("r", "r", "sub, dom, obj, act")

	// Policy definition
	m.AddDef("p", "p", "sub, dom, obj, act")

	// Role definition
	m.AddDef("g", "g", "_, _")

	// Policy effect
	m.AddDef("e", "e", "some(where (p.eft == allow))")

	// Matchers
	m.AddDef("m", "m", "g(r.sub, p.sub) && r.dom == p.dom && keyMatchEcho(r.obj, p.obj) && r.act == p.act")

	db := a.DB()

	// Load policies from database
	gormadapter.TurnOffAutoMigrate(db)
	tbl := models.AccessRule{}
	tableName := db.NamingStrategy.TableName(reflect.TypeOf(tbl).Name())
	adapter, err := gormadapter.NewAdapterByDBWithCustomTable(db, &tbl, tableName)
	if err != nil {
		return err
	}

	// Load the model with adapter
	enforcer, err := casbin.NewEnforcer(m, adapter)
	if err != nil {
		return err
	}

	// Register custom key matcher for Echo's colon syntax
	// Must be registered before the enforcer is used
	enforcer.AddFunction("keyMatchEcho", access.KeyMatchEchoFunc)

	enforcer.EnableAutoSave(true)

	a.enforcer = enforcer

	return nil
}

func (a *AccessServiceDefault) GetEnforcer() *casbin.Enforcer {
	return a.enforcer
}

func (a *AccessServiceDefault) ExportModel(context.Context) *core.AccessModel {
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

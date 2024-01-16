package api

import (
	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	"github.com/casbin/casbin/v2/persist"
	"go.uber.org/zap"
	"strings"
	"sync"
)

func GetCasbin(logger *zap.Logger) *casbin.Enforcer {
	m := model.NewModel()
	m.AddDef("r", "r", "sub, obj, act")
	m.AddDef("p", "p", "sub, obj, act")
	m.AddDef("g", "g", "_, _")
	m.AddDef("e", "e", "some(where (p.eft == allow))")
	m.AddDef("m", "m", "g(r.sub, p.sub) && r.obj == p.obj && r.act == p.act")

	a := NewPolicyAdapter(logger)

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		logger.Fatal("Failed to create casbin enforcer", zap.Error(err))
	}

	// Add policies after creating the enforcer
	_ = a.AddPolicy("admin", "/admin", []string{"GET"})
	_ = a.AddPolicy("admin", "/admin", []string{"POST"})
	_ = a.AddPolicy("admin", "/admin", []string{"DELETE"})

	err = e.LoadPolicy()
	if err != nil {
		logger.Fatal("Failed to load policies into Casbin model", zap.Error(err))
	}

	return e
}

type PolicyAdapter struct {
	policy []string
	lock   sync.RWMutex
	logger *zap.Logger
}

// NewPolicyAdapter creates a new PolicyAdapter instance.
func NewPolicyAdapter(logger *zap.Logger) *PolicyAdapter {
	return &PolicyAdapter{
		policy: make([]string, 0),
		logger: logger,
	}
}

// LoadPolicy loads all policy rules from the storage.
func (a *PolicyAdapter) LoadPolicy(model model.Model) error {
	a.lock.RLock()
	defer a.lock.RUnlock()

	for _, line := range a.policy {
		err := persist.LoadPolicyLine(line, model)
		if err != nil {
			a.logger.Fatal("Failed to load policy line", zap.Error(err))
		}
	}
	return nil
}

// SavePolicy saves all policy rules to the storage.
func (a *PolicyAdapter) SavePolicy(model model.Model) error {
	return nil
}

// AddPolicy adds a policy rule to the storage.
// AddPolicy adds a policy rule to the storage.
func (a *PolicyAdapter) AddPolicy(sec string, ptype string, rule []string) error {
	a.lock.Lock()
	defer a.lock.Unlock()

	// Create a line representing the policy rule with the section
	line := sec + ", " + ptype + ", " + strings.Join(rule, ", ")

	// Check if the policy rule already exists
	for _, existingLine := range a.policy {
		if line == existingLine {
			return nil // Policy rule already exists, no need to add it again
		}
	}

	// Add the policy rule to the storage
	a.policy = append(a.policy, line)
	return nil
}

// RemovePolicy removes a policy rule from the storage.
func (a *PolicyAdapter) RemovePolicy(sec string, ptype string, rule []string) error {
	return nil
}

// RemoveFilteredPolicy removes policy rules that match the filter from the storage.
func (a *PolicyAdapter) RemoveFilteredPolicy(sec string, ptype string, fieldIndex int, fieldValues ...string) error {
	return nil
}

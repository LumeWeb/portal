package api

import (
	"embed"
	_ "embed"
	"os"

	"github.com/casbin/casbin/v2"
	casbinfsadapter "github.com/naucon/casbin-fs-adapter"
	"go.uber.org/zap"
)

//go:embed casbin/*
var casbinData embed.FS

func NewCasbin(logger *zap.Logger) (*casbin.Enforcer, error) {
	fsys := os.DirFS("casbin")
	a := casbinfsadapter.NewAdapter(fsys, "policy.csv")
	m, err := casbinfsadapter.NewModel(fsys, "model.conf")

	if err != nil {
		return nil, err
	}

	e, err := casbin.NewEnforcer(m, a)
	if err != nil {
		logger.Fatal("Failed to create casbin enforcer", zap.Error(err))
	}

	// Add policies after creating the enforcer
	_ = a.AddPolicy("p", "p", []string{"admin", "/admin*"})

	err = e.LoadPolicy()
	if err != nil {
		logger.Fatal("Failed to load policies into Casbin model", zap.Error(err))
	}

	return e, nil
}

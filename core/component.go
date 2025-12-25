package core

import (
	"go.lumeweb.com/portal/config"
	"gorm.io/gorm"
)

type Component interface {
	ID() string
	Context() Context
	SetContext(ctx Context)
	Logger() *Logger
	SetLogger(logger *Logger)
	DB() *gorm.DB
	SetDB(db *gorm.DB)
	Config() config.Manager
	SetConfig(cfg config.Manager)
}

// BaseComponent provides a base implementation for Service with context, logger, and config management
type BaseComponent struct {
	ctx    Context
	logger *Logger
	db     *gorm.DB
	cfg    config.Manager
}

// NewBaseComponent creates a new BaseComponent with the given context
func NewBaseComponent(ctx Context) *BaseComponent {
	return &BaseComponent{
		ctx:    ctx,
		logger: ctx.Logger(),
		db:     ctx.DB(),
		cfg:    ctx.Config(),
	}
}

// Context returns the service's context
func (bc *BaseComponent) Context() Context {
	return bc.ctx
}

// SetContext sets the service's context
func (bc *BaseComponent) SetContext(ctx Context) {
	bc.ctx = ctx
}

// Logger returns the service's logger
func (bc *BaseComponent) Logger() *Logger {
	return bc.logger
}

// SetLogger sets the service's logger
func (bc *BaseComponent) SetLogger(logger *Logger) {
	bc.logger = logger
}

// DB returns the service's database
func (bc *BaseComponent) DB() *gorm.DB {
	return bc.db
}

// SetDB sets the service's database
func (bc *BaseComponent) SetDB(db *gorm.DB) {
	bc.db = db
}

// Config returns the component's config manager
func (bc *BaseComponent) Config() config.Manager {
	return bc.cfg
}

// SetConfig sets the component's config manager
func (bc *BaseComponent) SetConfig(cfg config.Manager) {
	bc.cfg = cfg
}

package db

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/core"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// SQLiteMemoryProvider implements a shared in-memory SQLite database provider
type SQLiteMemoryProvider struct {
	db     *gorm.DB
	mu     sync.Mutex
	config config.Manager
	ctx    core.Context
}

// NewSQLiteMemoryProvider creates a new in-memory SQLite provider
func NewSQLiteMemoryProvider(ctx core.Context) *SQLiteMemoryProvider {
	return &SQLiteMemoryProvider{
		config: ctx.Config(),
		ctx:    ctx,
	}
}

// Connect establishes or returns an existing connection to the shared in-memory database
func (p *SQLiteMemoryProvider) Connect(logger *core.Logger) (*gorm.DB, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db != nil {
		return p.db, nil
	}

	// Get DSN using centralized helper
	dsn, err := GetDSN(p.config)
	if err != nil {
		return nil, err
	}

	_db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger:  NewLogger(logger.Logger, logger.Level()),
		NowFunc: func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, err
	}

	p.db = _db
	return _db, nil
}

// Close cleanly shuts down the database connection
func (p *SQLiteMemoryProvider) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.db == nil {
		return nil
	}

	sqlDB, err := p.db.DB()
	if err != nil {
		return err
	}

	err = sqlDB.Close()
	p.db = nil
	return err
}

// GetDialector returns the SQLite dialector configured for shared in-memory mode
func (p *SQLiteMemoryProvider) GetDialector() gorm.Dialector {
	dsn, err := GetDSN(p.config)
	if err != nil {
		p.ctx.Logger().Fatal("Failed to get database DSN", zap.Error(err))
	}
	return sqlite.Open(dsn)
}

package tus

import (
	"context"
	"github.com/tus/tusd/v2/pkg/handler"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db/models"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"os"
	"sync"
	"time"
)

var (
	_ handler.Locker = (*DbLocker)(nil)
	_ handler.Lock   = (*Lock)(nil)
)

type DbLocker struct {
	AcquirerPollInterval time.Duration
	HolderPollInterval   time.Duration
	db                   *gorm.DB
	logger               *core.Logger
}

type Lock struct {
	locker               *DbLocker
	id                   string
	holderPollInterval   time.Duration
	acquirerPollInterval time.Duration
	stopHolderPoll       chan struct{}
	lockRecord           models.TusLock
	once                 sync.Once
}

func NewDbLocker(db *gorm.DB, logger *core.Logger) *DbLocker {
	return &DbLocker{HolderPollInterval: 5 * time.Second, AcquirerPollInterval: 2 * time.Second, db: db, logger: logger}
}

func (l *Lock) released() error {
	err := l.lockRecord.Released(l.locker.db)
	if err != nil {
		l.locker.logger.Error("Failed to release lock", zap.Error(err))
		return err
	}

	return nil
}
func (l *Lock) Lock(ctx context.Context, requestUnlock func()) error {

	db := l.locker.db

	for {
		err := l.lockRecord.TryLock(db, ctx)

		if err == nil {
			break
		}

		if err != models.ErrTusLockBusy {
			return err
		}

		err = l.lockRecord.RequestRelease(db)
		if err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			err := l.released()
			if err != nil {
				return err
			}
			// Context expired, so we return a timeout
			return handler.ErrLockTimeout
		case <-time.After(l.acquirerPollInterval):
			// Continue with the next attempt after a short delay
			continue
		}
	}

	go func() {
		ticker := time.NewTicker(l.holderPollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-l.stopHolderPoll:
				return
			case <-ticker.C:
				requested, err := l.lockRecord.IsReleaseRequested(db)
				if err != nil {
					// Handle error
					continue
				}

				if requested {
					requestUnlock()
					return
				}
			}
		}
	}()

	return nil

}

func (l *Lock) Unlock() error {
	l.once.Do(func() {
		close(l.stopHolderPoll)
	})

	return l.lockRecord.Delete(l.locker.db)
}

func (m *DbLocker) NewLock(id string) (handler.Lock, error) {
	return &Lock{
		locker:               m,
		id:                   id,
		holderPollInterval:   m.HolderPollInterval,
		acquirerPollInterval: m.AcquirerPollInterval,
		stopHolderPoll:       make(chan struct{}),
		lockRecord: models.TusLock{
			LockId:     id,
			HolderPID:  os.Getpid(),
			AcquiredAt: time.Now(),
			ExpiresAt:  time.Now().Add(30 * time.Minute),
		},
	}, nil
}

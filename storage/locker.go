package storage

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"os"
	"sync"
	"time"
)

var (
	_ tusd.Locker = (*MySQLLocker)(nil)
	_ tusd.Lock   = (*Lock)(nil)
)

type MySQLLocker struct {
	storage              interfaces.StorageService
	AcquirerPollInterval time.Duration
	HolderPollInterval   time.Duration
}

type Lock struct {
	locker               *MySQLLocker
	id                   string
	holderPollInterval   time.Duration
	acquirerPollInterval time.Duration
	stopHolderPoll       chan struct{}
	lockRecord           models.TusLock
	once                 sync.Once
}

func NewMySQLLocker(storage interfaces.StorageService) *MySQLLocker {
	return &MySQLLocker{storage: storage, HolderPollInterval: 5 * time.Second, AcquirerPollInterval: 2 * time.Second}
}

func (l *Lock) Lock(ctx context.Context, requestUnlock func()) error {

	db := l.locker.storage.Portal().Database()

	for {
		err := l.lockRecord.TryLock(db, ctx)

		if err == nil {
			break
		}

		if err != models.ErrTusLockBusy {
			return err
		}

		select {
		case <-ctx.Done():
			// Context expired, so we return a timeout
			return tusd.ErrLockTimeout
		case <-time.After(l.acquirerPollInterval):
			// Continue with the next attempt after a short delay
			continue
		}
	}

	err := l.lockRecord.RequestRelease(db)
	if err != nil {
		return err
	}

	defer func(lockRecord *models.TusLock, db *gorm.DB) {
		err := lockRecord.Released(db)
		if err != nil {
			l.locker.storage.Portal().Logger().Error("Failed to release lock", zap.Error(err))
		}
	}(&l.lockRecord, db)

	go func() {
		for {
			select {
			case <-l.stopHolderPoll:
				return
			case <-time.After(l.holderPollInterval):
				requested, err := l.lockRecord.IsReleaseRequested(db)
				if err == nil {
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
	close(l.stopHolderPoll)
	l.once.Do(func() {
		close(l.stopHolderPoll)
	})

	return l.lockRecord.Delete(l.locker.storage.Portal().Database())
}

func (m *MySQLLocker) NewLock(id string) (tusd.Lock, error) {
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

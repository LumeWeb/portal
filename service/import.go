package service

import (
	"context"
	"errors"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

var _ core.ImportService = (*ImportServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.IMPORT_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewImportService()
		},
	})
}

type ImportServiceDefault struct {
	ctx core.Context
	db  *gorm.DB
}

func NewImportService() (*ImportServiceDefault, []core.ContextBuilderOption, error) {
	_import := ImportServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_import.ctx = ctx
			_import.db = ctx.DB()
			return nil
		}),
	)

	return &_import, opts, nil
}

func (i ImportServiceDefault) UpdateProgress(ctx context.Context, objectHash []byte, stage int, totalStages int) error {
	_import, err := i.GetImport(ctx, objectHash)
	if err != nil {
		return err
	}

	if _import.IsEmpty() {
		return ErrNotFound
	}

	_import.Progress = float64(stage) / float64(totalStages) * 100.0

	return i.SaveImport(ctx, _import, false)
}

func (i ImportServiceDefault) UpdateStatus(ctx context.Context, objectHash []byte, status models.ImportStatus) error {
	_import, err := i.GetImport(ctx, objectHash)
	if err != nil {
		return err
	}

	if _import.IsEmpty() {
		return ErrNotFound
	}

	_import.Status = status

	return i.SaveImport(ctx, _import, false)
}

func (i ImportServiceDefault) SaveImport(ctx context.Context, metadata core.ImportMetadata, skipExisting bool) error {
	var __import models.Import

	__import.Hash = metadata.Hash

	if err := db.RetryOnLock(i.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.Import{}).Where(&__import).First(&__import)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return i.createImport(ctx, metadata)
		}
		return err
	}

	if skipExisting {
		return nil
	}

	changed := false

	if __import.UserID != metadata.UserID {
		__import.UserID = metadata.UserID
		changed = true
	}

	if __import.Status != metadata.Status {
		__import.Status = metadata.Status
		changed = true
	}

	if __import.Progress != metadata.Progress {
		__import.Progress = metadata.Progress
		changed = true
	}

	if __import.Protocol != metadata.Protocol {
		__import.Protocol = metadata.Protocol
		changed = true
	}

	if __import.ImporterIP != metadata.ImporterIP {
		__import.ImporterIP = metadata.ImporterIP
		changed = true
	}
	if changed {
		return i.db.Updates(&__import).Error
	}

	return nil
}

func (m *ImportServiceDefault) createImport(ctx context.Context, metadata core.ImportMetadata) error {
	__import := models.Import{
		UserID:     metadata.UserID,
		Hash:       metadata.Hash,
		Status:     metadata.Status,
		Progress:   metadata.Progress,
		Protocol:   metadata.Protocol,
		ImporterIP: metadata.ImporterIP,
	}

	if __import.Status == "" {
		__import.Status = models.ImportStatusQueued
	}

	return m.db.WithContext(ctx).Create(&__import).Error
}

func (i ImportServiceDefault) GetImport(ctx context.Context, objectHash []byte) (core.ImportMetadata, error) {
	var _import models.Import

	_import.Hash = objectHash

	if err := db.RetryOnLock(i.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.Import{}).Where(&_import).First(&_import)

	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return core.ImportMetadata{}, ErrNotFound
		}
		return core.ImportMetadata{}, err
	}

	return core.ImportMetadata{
		ID:         _import.ID,
		UserID:     _import.UserID,
		Hash:       _import.Hash,
		Protocol:   _import.Protocol,
		Status:     _import.Status,
		Progress:   _import.Progress,
		ImporterIP: _import.ImporterIP,
		Created:    _import.CreatedAt,
	}, nil
}

func (i ImportServiceDefault) DeleteImport(ctx context.Context, objectHash []byte) error {
	var _import models.Import

	_import.Hash = objectHash

	if err := db.RetryOnLock(i.db, func(db *gorm.DB) *gorm.DB {
		return db.WithContext(ctx).Model(&models.Import{}).Where(&_import).First(&_import)
	}); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return err
	}

	return nil
}

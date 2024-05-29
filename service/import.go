package service

import (
	"context"
	"errors"
	"github.com/LumeWeb/portal/core"
	"github.com/LumeWeb/portal/db/models"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

var _ core.ImportService = (*ImportServiceDefault)(nil)

type ImportServiceDefault struct {
	ctx *core.Context
	db  *gorm.DB
}

func NewImportService(ctx *core.Context) *ImportServiceDefault {
	_import := ImportServiceDefault{
		ctx: ctx,
		db:  ctx.DB(),
	}

	ctx.RegisterService(_import)

	return &_import
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

	ret := i.db.WithContext(ctx).Model(&models.Import{}).Where(&__import).First(&__import)

	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return i.createImport(ctx, metadata)
		}
		return ret.Error
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

	ret := i.db.WithContext(ctx).Model(&models.Import{}).Where(&_import).First(&_import)

	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return core.ImportMetadata{}, ErrNotFound
		}
		return core.ImportMetadata{}, ret.Error

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

	ret := i.db.WithContext(ctx).Model(&models.Import{}).Where(&_import).Delete(&_import)

	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return ret.Error
	}

	return nil
}

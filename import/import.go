package _import

import (
	"context"
	"errors"
	"io"
	"time"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"go.uber.org/fx"
	"gorm.io/gorm"
)

var ErrNotFound = gorm.ErrRecordNotFound

var _ ImportService = (*ImportServiceDefault)(nil)
var _ io.Reader = (*ImportReader)(nil)

type ImportMetadata struct {
	ID         uint
	UserID     uint
	Hash       []byte
	Status     models.ImportStatus
	Progress   float64
	Protocol   string
	ImporterIP string
	Created    time.Time
}

type ImportService interface {
	SaveImport(ctx context.Context, metadata ImportMetadata, skipExisting bool) error
	GetImport(ctx context.Context, objectHash []byte) (ImportMetadata, error)
	DeleteImport(ctx context.Context, objectHash []byte) error
}

func (u ImportMetadata) IsEmpty() bool {
	if u.UserID != 0 || u.Protocol != "" || u.ImporterIP != "" || u.Status != "" {
		return false
	}

	if !u.Created.IsZero() {
		return false
	}

	if len(u.Hash) != 0 {
		return false
	}

	return true
}

var Module = fx.Module("import",
	fx.Provide(
		fx.Annotate(
			NewImportService,
			fx.As(new(ImportService)),
		),
	),
)

type ImportServiceDefault struct {
	db *gorm.DB
}

func (i ImportServiceDefault) SaveImport(ctx context.Context, metadata ImportMetadata, skipExisting bool) error {
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
		changed = true
	}

	if __import.Status != metadata.Status {
		changed = true
	}

	if __import.Progress != metadata.Progress {
		changed = true
	}

	if __import.Protocol != metadata.Protocol {
		changed = true
	}

	if __import.ImporterIP != metadata.ImporterIP {
		changed = true
	}
	if changed {
		return i.db.Updates(&__import).Error
	}

	return nil
}

func (m *ImportServiceDefault) createImport(ctx context.Context, metadata ImportMetadata) error {
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

func (i ImportServiceDefault) GetImport(ctx context.Context, objectHash []byte) (ImportMetadata, error) {
	var _import models.Import

	_import.Hash = objectHash

	ret := i.db.WithContext(ctx).Model(&models.Import{}).Where(&_import).First(&_import)

	if ret.Error != nil {
		if errors.Is(ret.Error, gorm.ErrRecordNotFound) {
			return ImportMetadata{}, ErrNotFound
		}
		return ImportMetadata{}, ret.Error

	}

	return ImportMetadata{
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

type ImportServiceParams struct {
	fx.In
	Db *gorm.DB
}

func NewImportService(params ImportServiceParams) *ImportServiceDefault {
	return &ImportServiceDefault{
		db: params.Db,
	}
}

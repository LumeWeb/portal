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
var _ io.ReadSeekCloser = (*ImportReader)(nil)

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

type ImportReader struct {
	service     ImportService
	meta        ImportMetadata
	reader      io.Reader
	size        uint64
	stage       int
	totalStages int
	bytesRead   uint64
}

func (i *ImportReader) Seek(offset int64, whence int) (int64, error) {
	if seeker, ok := i.reader.(io.Seeker); ok {
		// If seeking to the start, reset progress based on recorded bytes
		if whence == io.SeekStart && offset == 0 {
			i.bytesRead = 0
			i.meta.Progress = 0
			if err := i.service.SaveImport(context.Background(), i.meta, false); err != nil {
				return 0, err
			}
		}
		return seeker.Seek(offset, whence)
	}

	return 0, errors.New("Seek not supported")
}

func (i *ImportReader) Close() error {
	if closer, ok := i.reader.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

func (i *ImportReader) Read(p []byte) (n int, err error) {
	n, err = i.reader.Read(p)
	if err != nil {
		return 0, err
	}

	// Update cumulative bytes read
	i.bytesRead += uint64(n)

	err = i.ReadBytes(n)
	if err != nil {
		return 0, err
	}

	return n, nil
}

func (i *ImportReader) ReadBytes(n int) (err error) {
	stageProgress := float64(100) / float64(i.totalStages)

	// Calculate progress based on bytes read
	i.meta.Progress = float64(i.bytesRead) / float64(i.size) * 100.0

	// Adjust progress for current stage
	if i.stage > 1 {
		i.meta.Progress += float64(i.stage-1) * stageProgress
	}

	// Ensure progress doesn't exceed 100%
	if i.meta.Progress > 100 {
		i.meta.Progress = 100
	}

	// Save import progress
	err = i.service.SaveImport(context.Background(), i.meta, false)
	if err != nil {
		return err
	}

	return nil
}

func NewImportReader(service ImportService, meta ImportMetadata, reader io.Reader, size uint64, stage, totalStages int) *ImportReader {
	return &ImportReader{
		service:     service,
		meta:        meta,
		reader:      reader,
		size:        size,
		stage:       stage,
		totalStages: totalStages,
	}
}

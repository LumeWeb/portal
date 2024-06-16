package core

import (
	"context"
	"go.lumeweb.com/portal/db/models"
	"time"
)

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

type ImportService interface {
	SaveImport(ctx context.Context, metadata ImportMetadata, skipExisting bool) error
	GetImport(ctx context.Context, objectHash []byte) (ImportMetadata, error)
	DeleteImport(ctx context.Context, objectHash []byte) error
	UpdateProgress(ctx context.Context, objectHash []byte, stage int, totalStages int) error
	UpdateStatus(ctx context.Context, objectHash []byte, status models.ImportStatus) error
}

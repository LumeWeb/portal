package service

import (
	"context"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	"gorm.io/gorm"
)

var _ core.UploadDataHandler = (*PostDataHandler)(nil)
var _ core.UploadDataHandler = (*TUSDataHandler)(nil)

func init() {
	core.RegisterUploadDataHandler("post", &PostDataHandler{})
	core.RegisterUploadDataHandler("tus", &TUSDataHandler{})
}

type PostDataHandler struct {
}

func (p *PostDataHandler) CreateUploadData(_ context.Context, _ *gorm.DB, _ uint, _ any) error {
	return nil
}

func (p *PostDataHandler) GetUploadData(_ context.Context, _ *gorm.DB, _ uint) (any, error) {
	return nil, nil
}

func (p *PostDataHandler) UpdateUploadData(_ context.Context, _ *gorm.DB, _ uint, _ any) error {
	return nil
}

func (p *PostDataHandler) DeleteUploadData(_ context.Context, _ *gorm.DB, _ uint) error {
	return nil
}

func (p *PostDataHandler) QueryUploadData(_ context.Context, tx *gorm.DB, _ any) *gorm.DB {
	return tx
}

func (p *PostDataHandler) CompleteUploadData(_ context.Context, _ *gorm.DB, _ uint) error {
	return nil
}

func (p *PostDataHandler) GetUploadDataModel() any {
	return nil
}

type TUSDataHandler struct {
}

func (T TUSDataHandler) CreateUploadData(ctx context.Context, tx *gorm.DB, id uint, data any) error {
	uploadData := data.(*models.TUSRequest)
	uploadData.RequestID = id

	return tx.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).FirstOrCreate(uploadData, &models.TUSRequest{
				RequestID: id,
			})
		})
	})
}

func (T TUSDataHandler) GetUploadData(ctx context.Context, tx *gorm.DB, id uint) (any, error) {
	uploadData := &models.TUSRequest{
		RequestID: id,
	}
	err := tx.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where(uploadData).First(uploadData)
		})
	})
	if err != nil {
		return nil, err
	}

	return uploadData, nil
}

func (T TUSDataHandler) UpdateUploadData(ctx context.Context, tx *gorm.DB, id uint, data any) error {
	uploadData := data.(*models.TUSRequest)
	uploadData.RequestID = id

	return tx.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Save(uploadData)
		})
	})
}

func (T TUSDataHandler) DeleteUploadData(ctx context.Context, tx *gorm.DB, id uint) error {
	uploadData := &models.TUSRequest{}
	err := tx.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Delete(uploadData, id)
		})
	})
	if err != nil {
		return err
	}

	return nil
}

func (T TUSDataHandler) QueryUploadData(ctx context.Context, tx *gorm.DB, query any) *gorm.DB {
	return tx.Where(query)
}

func (T TUSDataHandler) CompleteUploadData(ctx context.Context, tx *gorm.DB, id uint) error {
	uploadData := &models.TUSRequest{
		RequestID: id,
	}
	err := tx.Transaction(func(tx *gorm.DB) error {
		return db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
			return db.WithContext(ctx).Where(uploadData).Delete(uploadData)
		})
	})
	if err != nil {
		return err
	}

	return nil
}

func (T TUSDataHandler) GetUploadDataModel() any {
	return &models.TUSRequest{}
}

package service

import (
	"context"
	"errors"
	"fmt"

	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/db"
	"go.lumeweb.com/portal/db/models"
	uploadMetrics "go.lumeweb.com/portal/service/internal/upload"
	"gorm.io/gorm"
)

var _ core.UploadService = (*UploadServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID:      core.UPLOAD_SERVICE,
		Factory: NewMetadataService,
		Metrics: uploadMetrics.GetCollectors(),
	})
}

type UploadServiceDefault struct {
	*core.BaseComponent
}

func NewMetadataService() (core.Service, []core.ContextBuilderOption, error) {
	meta := &UploadServiceDefault{}

	return meta, nil, nil
}

func (m *UploadServiceDefault) ID() string {
	return core.UPLOAD_SERVICE
}

func (m *UploadServiceDefault) SaveUpload(ctx context.Context, upload *models.Upload) error {
	ctx, span := core.TraceMethod(ctx, "UploadServiceDefault.SaveUpload")
	defer span.End()

	return core.MetricTrack(
		uploadMetrics.UploadDuration.WithLabelValues(uploadMetrics.LabelOpSave),
		uploadMetrics.UploadFailed.WithLabelValues(uploadMetrics.LabelOpSave),
		func() error {
			err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
				existingUpload := &models.Upload{
					Hash:     upload.Hash,
					Protocol: upload.Protocol,
				}

				err := db.RetryOnLock(tx, func(db *gorm.DB) *gorm.DB {
					return db.Model(existingUpload).Where(existingUpload).First(existingUpload)
				})

				if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
					_ = tx.AddError(err)
					return tx
				}

				// If the record doesn't exist, create a new one
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return tx.Create(upload)
				}

				// Update fields if they are different and not empty
				if upload.UserID != 0 && upload.UserID != existingUpload.UserID {
					existingUpload.UserID = upload.UserID
				}
				if upload.MimeType != "" && upload.MimeType != existingUpload.MimeType {
					existingUpload.MimeType = upload.MimeType
				}
				if upload.UploaderIP != "" && upload.UploaderIP != existingUpload.UploaderIP {
					existingUpload.UploaderIP = upload.UploaderIP
				}
				if upload.Size != 0 && upload.Size != existingUpload.Size {
					existingUpload.Size = upload.Size
				}

				save := tx.Save(existingUpload)

				if err = save.Error; err == nil {
					upload.ID = existingUpload.ID
				}

				return save
			})

			if err == nil {
				uploadMetrics.UploadsSaved.WithLabelValues(uploadMetrics.LabelOpSave).Inc()
			}
			return err
		},
	)
}

func (m *UploadServiceDefault) GetUpload(ctx context.Context, objectHash core.StorageHash) (*models.Upload, error) {
	ctx, span := core.TraceMethod(ctx, "UploadServiceDefault.GetUpload")
	defer span.End()

	var upload models.Upload
	upload.Hash = objectHash.Multihash()

	if upload.Hash == nil || len(upload.Hash) == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	result, err := core.MetricTrackResult(
		uploadMetrics.UploadDuration.WithLabelValues(uploadMetrics.LabelOpGet),
		uploadMetrics.UploadFailed.WithLabelValues(uploadMetrics.LabelOpGet),
		func() (*models.Upload, error) {
			if err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&upload).Where(&upload).First(&upload)
			}); err != nil {
				return nil, err
			}
			return &upload, nil
		},
	)

	if err == nil {
		uploadMetrics.UploadsQueried.WithLabelValues(uploadMetrics.LabelOpGet).Inc()
	}
	return result, err
}

func (m *UploadServiceDefault) DeleteUpload(ctx context.Context, objectHash core.StorageHash) error {
	ctx, span := core.TraceMethod(ctx, "UploadServiceDefault.DeleteUpload")
	defer span.End()

	var upload models.Upload
	upload.Hash = objectHash.Multihash()

	return core.MetricTrack(
		uploadMetrics.UploadDuration.WithLabelValues(uploadMetrics.LabelOpDelete),
		uploadMetrics.UploadFailed.WithLabelValues(uploadMetrics.LabelOpDelete),
		func() error {
			if err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&upload).Where(&upload).First(&upload)
			}); err != nil {
				return err
			}

			if err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Delete(&upload)
			}); err != nil {
				return err
			}

			uploadMetrics.UploadsDeleted.WithLabelValues(uploadMetrics.LabelOpDelete).Inc()
			return nil
		},
	)
}

func (m *UploadServiceDefault) GetAllUploads(ctx context.Context) ([]*models.Upload, error) {
	ctx, span := core.TraceMethod(ctx, "UploadServiceDefault.GetAllUploads")
	defer span.End()

	result, err := core.MetricTrackResult(
		uploadMetrics.UploadDuration.WithLabelValues(uploadMetrics.LabelOpListAll),
		uploadMetrics.UploadFailed.WithLabelValues(uploadMetrics.LabelOpListAll),
		func() ([]*models.Upload, error) {
			var uploads []*models.Upload

			if err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Find(&uploads)
			}); err != nil {
				return nil, err
			}

			return uploads, nil
		},
	)

	if err == nil {
		uploadMetrics.UploadsListed.WithLabelValues(uploadMetrics.LabelOpListAll).Inc()
	}
	return result, err
}

func (m *UploadServiceDefault) GetUploadByID(ctx context.Context, uploadID uint) (*models.Upload, error) {
	ctx, span := core.TraceMethod(ctx, "UploadServiceDefault.GetUploadByID")
	defer span.End()

	var upload models.Upload
	upload.ID = uploadID

	result, err := core.MetricTrackResult(
		uploadMetrics.UploadDuration.WithLabelValues(uploadMetrics.LabelOpGetByID),
		uploadMetrics.UploadFailed.WithLabelValues(uploadMetrics.LabelOpGetByID),
		func() (*models.Upload, error) {
			if err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
				return tx.Model(&models.Upload{}).Where(&upload).First(&upload)
			}); err != nil {
				return nil, err
			}
			return &upload, nil
		},
	)

	if err == nil {
		uploadMetrics.UploadsQueried.WithLabelValues(uploadMetrics.LabelOpGetByID).Inc()
	}
	return result, err
}

func (m *UploadServiceDefault) GetUploadStats(ctx context.Context) ([]core.ProtocolUploadStat, error) {
	ctx, span := core.TraceMethod(ctx, "UploadServiceDefault.GetUploadStats")
	defer span.End()

	var stats []core.ProtocolUploadStat

	if err := db.RetryableComponentTransaction(m, ctx, func(tx *gorm.DB) *gorm.DB {
		return tx.Table("uploads").
			Select("protocol, COUNT(DISTINCT id) as total_uploads, COALESCE(SUM(size), 0) as total_storage_bytes").
			Where("deleted_at IS NULL").
			Group("protocol").
			Scan(&stats)
	}); err != nil {
		return nil, fmt.Errorf("failed to get upload stats: %w", err)
	}

	return stats, nil
}

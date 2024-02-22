package s5

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/config"

	"go.uber.org/fx"

	"git.lumeweb.com/LumeWeb/portal/account"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/tus/tusd/v2/pkg/s3store"

	tusd "github.com/tus/tusd/v2/pkg/handler"

	"git.lumeweb.com/LumeWeb/portal/storage"
	"gorm.io/gorm"

	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/portal/cron"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/renter"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

var (
	_ cron.CronableService = (*TusHandler)(nil)
)

type TusHandler struct {
	config          *config.Manager
	db              *gorm.DB
	logger          *zap.Logger
	cron            *cron.CronServiceDefault
	storage         storage.StorageService
	accounts        *account.AccountServiceDefault
	metadata        metadata.MetadataService
	tus             *tusd.Handler
	tusStore        tusd.DataStore
	s3Client        *s3.Client
	storageProtocol storage.StorageProtocol
}

type TusHandlerParams struct {
	fx.In
	Config   *config.Manager
	Logger   *zap.Logger
	Db       *gorm.DB
	Cron     *cron.CronServiceDefault
	Storage  storage.StorageService
	Accounts *account.AccountServiceDefault
	Metadata metadata.MetadataService
}

func NewTusHandler(lc fx.Lifecycle, params TusHandlerParams) *TusHandler {
	th := &TusHandler{
		config:   params.Config,
		db:       params.Db,
		logger:   params.Logger,
		cron:     params.Cron,
		storage:  params.Storage,
		accounts: params.Accounts,
		metadata: params.Metadata,
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			go th.worker()
			return nil
		},
	})

	return th
}

func (t *TusHandler) Init() error {
	preUpload := func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
		blankResp := tusd.HTTPResponse{}
		blankChanges := tusd.FileInfoChanges{}

		hash, ok := hook.Upload.MetaData["hash"]
		if !ok {
			return blankResp, blankChanges, errors.New("missing hash")
		}

		decodedHash, err := encoding.MultihashFromBase64Url(hash)

		if err != nil {
			return blankResp, blankChanges, err
		}

		upload, err := t.metadata.GetUpload(hook.Context, decodedHash.HashBytes())

		if !upload.IsEmpty() {
			if err != nil && !errors.Is(err, metadata.ErrNotFound) {
				return blankResp, blankChanges, err
			}
			return blankResp, blankChanges, errors.New("file already exists")
		}

		exists, _ := t.UploadExists(hook.Context, decodedHash.HashBytes())

		if exists {
			return blankResp, blankChanges, errors.New("file is already being uploaded")
		}

		return blankResp, blankChanges, nil
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           t.config.Config().Core.Storage.S3.Endpoint,
				SigningRegion: t.config.Config().Core.Storage.S3.Region,
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := awsConfig.LoadDefaultConfig(context.TODO(),
		awsConfig.WithRegion("us-east-1"),
		awsConfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			t.config.Config().Core.Storage.S3.AccessKey,
			t.config.Config().Core.Storage.S3.SecretKey,
			"",
		)),
		awsConfig.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return err
	}

	s3Client := s3.NewFromConfig(cfg)

	store := s3store.New(t.config.Config().Core.Storage.S3.BufferBucket, s3Client)

	locker := NewMySQLLocker(t.db, t.logger)

	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	composer.UseLocker(locker)

	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:                "/s5/upload/tus",
		StoreComposer:           composer,
		DisableDownload:         true,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
		NotifyCreatedUploads:    true,
		RespectForwardedHeaders: true,
		PreUploadCreateCallback: preUpload,
	})

	if err != nil {
		return err
	}

	t.tus = handler
	t.tusStore = store
	t.s3Client = s3Client

	t.cron.RegisterService(t)
	return nil
}

func (t *TusHandler) LoadInitialTasks(cron cron.CronService) error {
	return nil
}

func (t *TusHandler) Tus() *tusd.Handler {
	return t.tus
}

func (t *TusHandler) UploadExists(ctx context.Context, hash []byte) (bool, models.TusUpload) {
	var upload models.TusUpload
	result := t.db.WithContext(ctx).Model(&models.TusUpload{}).Where(&models.TusUpload{Hash: hash}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (t *TusHandler) CreateUpload(ctx context.Context, hash []byte, uploadID string, uploaderID uint, uploaderIP string, protocol string) (*models.TusUpload, error) {
	upload := &models.TusUpload{
		Hash:       hash,
		UploadID:   uploadID,
		UploaderID: uploaderID,
		UploaderIP: uploaderIP,
		Uploader:   models.User{},
		Protocol:   protocol,
	}

	result := t.db.WithContext(ctx).Create(upload)

	if result.Error != nil {
		return nil, result.Error
	}

	return upload, nil
}
func (t *TusHandler) UploadProgress(ctx context.Context, uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := t.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = t.db.WithContext(ctx).Model(&models.TusUpload{}).Where(find).Update("updated_at", time.Now())

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (t *TusHandler) UploadCompleted(ctx context.Context, uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := t.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = t.db.WithContext(ctx).Model(&models.TusUpload{}).Where(find).Update("completed", true)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (t *TusHandler) DeleteUpload(ctx context.Context, uploadID string) error {
	result := t.db.WithContext(ctx).Where(&models.TusUpload{UploadID: uploadID}).Delete(&models.TusUpload{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (t *TusHandler) ScheduleUpload(ctx context.Context, uploadID string) error {
	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := t.db.WithContext(ctx).Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	task := t.cron.RetryableTask(cron.RetryableTaskParams{
		Name:     "tusUpload",
		Function: t.uploadTask,
		Args:     []interface{}{upload.Hash},
		Attempt:  0,
		Limit:    0,
		After: func(jobID uuid.UUID, jobName string) {
			t.logger.Info("Job finished", zap.String("jobName", jobName), zap.String("uploadID", uploadID))
			err := t.DeleteUpload(ctx, uploadID)
			if err != nil {
				t.logger.Error("Error deleting tus upload", zap.Error(err))
			}
		},
	})

	_, err := t.cron.CreateJob(task)

	if err != nil {
		return err
	}
	return nil
}

func (t *TusHandler) GetUploadReader(ctx context.Context, hash []byte, start int64) (io.ReadCloser, error) {
	exists, upload := t.UploadExists(ctx, hash)

	if !exists {
		return nil, metadata.ErrNotFound
	}

	meta, err := t.tusStore.GetUpload(ctx, upload.UploadID)
	if err != nil {
		return nil, err
	}

	info, err := meta.GetInfo(ctx)
	if err != nil {
		return nil, err
	}

	if start > 0 {
		endPosition := start + info.Size - 1
		rangeHeader := fmt.Sprintf("bytes=%d-%d", start, endPosition)
		ctx = context.WithValue(ctx, "range", rangeHeader)
	}

	reader, err := meta.GetReader(ctx)
	if err != nil {
		return nil, err
	}

	return reader, nil
}

func (t *TusHandler) SetStorageProtocol(storageProtocol storage.StorageProtocol) {
	t.storageProtocol = storageProtocol
}

func (t *TusHandler) uploadTask(hash []byte) error {
	ctx := context.Background()
	exists, upload := t.UploadExists(ctx, hash)

	if !exists {
		t.logger.Error("Upload not found", zap.String("hash", hex.EncodeToString(hash)))
		return metadata.ErrNotFound
	}

	tusUpload, err := t.tusStore.GetUpload(ctx, upload.UploadID)
	if err != nil {
		t.logger.Error("Could not get upload", zap.Error(err))
		return err
	}

	readers := make([]io.ReadCloser, 0)
	getReader := func() (io.Reader, error) {
		muReader, err := tusUpload.GetReader(ctx)
		if err != nil {
			return nil, err
		}

		readers = append(readers, muReader)
		return muReader, nil
	}

	defer func() {
		for _, reader := range readers {
			err := reader.Close()
			if err != nil {
				t.logger.Error("error closing reader", zap.Error(err))
			}
		}
	}()

	reader, err := getReader()
	if err != nil {
		t.logger.Error("Could not get tus file", zap.Error(err))
		return err
	}

	proof, err := t.storage.HashObject(ctx, reader)

	if err != nil {
		t.logger.Error("Could not compute proof", zap.Error(err))
		return err
	}

	if !bytes.Equal(proof.Hash, upload.Hash) {
		t.logger.Error("Hashes do not match", zap.Any("upload", upload), zap.Any("dbHash", hex.EncodeToString(upload.Hash)))
		return err
	}

	info, err := tusUpload.GetInfo(ctx)
	if err != nil {
		t.logger.Error("Could not get tus info", zap.Error(err))
		return err
	}

	uploadMeta, err := t.storage.UploadObject(ctx, t.storageProtocol, nil, &renter.MultiPartUploadParams{
		ReaderFactory: func(start uint, end uint) (io.ReadCloser, error) {
			rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
			ctx = context.WithValue(ctx, "range", rangeHeader)
			return tusUpload.GetReader(ctx)
		},
		Bucket:   upload.Protocol,
		FileName: t.storageProtocol.EncodeFileName(upload.Hash),
		Size:     uint64(info.Size),
	}, proof)

	if err != nil {
		t.logger.Error("Could not upload file", zap.Error(err))
		return err
	}

	s3InfoId, _ := splitS3Ids(upload.UploadID)

	_, err = t.s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(t.config.Config().Core.Storage.S3.BufferBucket),
		Delete: &s3types.Delete{
			Objects: []s3types.ObjectIdentifier{
				{
					Key: aws.String(s3InfoId),
				},
				{
					Key: aws.String(s3InfoId + ".info"),
				},
			},
			Quiet: aws.Bool(true),
		},
	})

	if err != nil {
		t.logger.Error("Could not delete upload metadata", zap.Error(err))
		return err
	}

	uploadMeta.UserID = upload.UploaderID
	uploadMeta.UploaderIP = upload.UploaderIP

	err = t.metadata.SaveUpload(ctx, *uploadMeta)
	if err != nil {
		t.logger.Error("Could not create upload", zap.Error(err))
		return err
	}

	err = t.accounts.PinByHash(upload.Hash, upload.UploaderID)
	if err != nil {
		t.logger.Error("Could not pin upload", zap.Error(err))
		return err
	}

	return nil
}

func (t *TusHandler) worker() {

	for {
		select {
		case info := <-t.tus.CreatedUploads:
			hash, ok := info.Upload.MetaData["hash"]
			errorResponse := tusd.HTTPResponse{StatusCode: 400, Header: nil}
			if !ok {
				t.logger.Error("Missing hash in metadata")
				continue
			}

			uploaderID, ok := info.Context.Value(middleware.DEFAULT_AUTH_CONTEXT_KEY).(uint)
			if !ok {
				errorResponse.Body = "Missing user id in context"
				info.Upload.StopUpload(errorResponse)
				t.logger.Error("Missing user id in context")
				continue
			}

			uploaderIP := info.HTTPRequest.RemoteAddr

			decodedHash, err := encoding.MultihashFromBase64Url(hash)

			if err != nil {
				errorResponse.Body = "Could not decode hash"
				info.Upload.StopUpload(errorResponse)
				t.logger.Error("Could not decode hash", zap.Error(err))
				continue
			}

			_, err = t.CreateUpload(info.Context, decodedHash.HashBytes(), info.Upload.ID, uploaderID, uploaderIP, t.storageProtocol.Name())
			if err != nil {
				errorResponse.Body = "Could not create tus upload"
				info.Upload.StopUpload(errorResponse)
				t.logger.Error("Could not create tus upload", zap.Error(err))
				continue
			}
		case info := <-t.tus.UploadProgress:
			err := t.UploadProgress(info.Context, info.Upload.ID)
			if err != nil {
				t.logger.Error("Could not update tus upload", zap.Error(err))
				continue
			}
		case info := <-t.tus.TerminatedUploads:
			err := t.DeleteUpload(info.Context, info.Upload.ID)
			if err != nil {
				t.logger.Error("Could not delete tus upload", zap.Error(err))
				continue
			}

		case info := <-t.tus.CompleteUploads:
			if !(!info.Upload.SizeIsDeferred && info.Upload.Offset == info.Upload.Size) {
				continue
			}
			err := t.UploadCompleted(info.Context, info.Upload.ID)
			if err != nil {
				t.logger.Error("Could not complete tus upload", zap.Error(err))
				continue
			}
			err = t.ScheduleUpload(info.Context, info.Upload.ID)
			if err != nil {
				t.logger.Error("Could not schedule tus upload", zap.Error(err))
				continue
			}

		}
	}
}

func splitS3Ids(id string) (objectId, multipartId string) {
	index := strings.Index(id, "+")
	if index == -1 {
		return
	}

	objectId = id[:index]
	multipartId = id[index+1:]
	return
}
func GetStorageProtocol(proto *S5Protocol) storage.StorageProtocol {
	return interface{}(proto).(storage.StorageProtocol)
}

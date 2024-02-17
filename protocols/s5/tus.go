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

	"git.lumeweb.com/LumeWeb/portal/account"

	"github.com/spf13/viper"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"github.com/aws/aws-sdk-go-v2/config"
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
	config   *viper.Viper
	db       *gorm.DB
	logger   *zap.Logger
	cron     *cron.CronServiceDefault
	storage  storage.StorageService
	accounts *account.AccountServiceDefault
	metadata metadata.MetadataService
	tus      *tusd.Handler
	tusStore tusd.DataStore
	s3Client *s3.Client
	protocol *S5Protocol
}

type TusHandlerParams struct {
	Config   *viper.Viper
	Logger   *zap.Logger
	Db       *gorm.DB
	Cron     *cron.CronServiceDefault
	Storage  storage.StorageService
	Accounts *account.AccountServiceDefault
	Metadata metadata.MetadataService
	Protocol *S5Protocol
}

func NewTusHandler(params TusHandlerParams) *TusHandler {
	return &TusHandler{
		config:   params.Config,
		db:       params.Db,
		logger:   params.Logger,
		cron:     params.Cron,
		storage:  params.Storage,
		accounts: params.Accounts,
		metadata: params.Metadata,
	}
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

		exists, _ := t.TusUploadExists(decodedHash.HashBytes())

		if exists {
			return blankResp, blankChanges, errors.New("file is already being uploaded")
		}

		return blankResp, blankChanges, nil
	}

	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           t.config.GetString("core.storage.s3.endpoint"),
				SigningRegion: t.config.GetString("core.storage.s3.region"),
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			t.config.GetString("core.storage.s3.accessKey"),
			t.config.GetString("core.storage.s3.secretKey"),
			"",
		)),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return err
	}

	s3Client := s3.NewFromConfig(cfg)

	store := s3store.New(t.config.GetString("core.storage.s3.bufferBucket"), s3Client)

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

func (t *TusHandler) TusUploadExists(hash []byte) (bool, models.TusUpload) {
	hashStr := hex.EncodeToString(hash)

	var upload models.TusUpload
	result := t.db.Model(&models.TusUpload{}).Where(&models.TusUpload{Hash: hashStr}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (t *TusHandler) CreateTusUpload(hash []byte, uploadID string, uploaderID uint, uploaderIP string, protocol string) (*models.TusUpload, error) {
	hashStr := hex.EncodeToString(hash)

	upload := &models.TusUpload{
		Hash:       hashStr,
		UploadID:   uploadID,
		UploaderID: uploaderID,
		UploaderIP: uploaderIP,
		Uploader:   models.User{},
		Protocol:   protocol,
	}

	result := t.db.Create(upload)

	if result.Error != nil {
		return nil, result.Error
	}

	return upload, nil
}
func (t *TusHandler) TusUploadProgress(uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := t.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = t.db.Model(&models.TusUpload{}).Where(find).Update("updated_at", time.Now())

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (t *TusHandler) TusUploadCompleted(uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := t.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = t.db.Model(&models.TusUpload{}).Where(find).Update("completed", true)

	return nil
}
func (t *TusHandler) DeleteTusUpload(uploadID string) error {
	result := t.db.Where(&models.TusUpload{UploadID: uploadID}).Delete(&models.TusUpload{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (t *TusHandler) ScheduleTusUpload(uploadID string) error {
	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := t.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	task := t.cron.RetryableTask(cron.RetryableTaskParams{
		Name:     "tusUpload",
		Function: t.tusUploadTask,
		Args:     []interface{}{&upload},
		Attempt:  0,
		Limit:    0,
		After: func(jobID uuid.UUID, jobName string) {
			t.logger.Info("Job finished", zap.String("jobName", jobName), zap.String("uploadID", uploadID))
			err := t.DeleteTusUpload(uploadID)
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

func (t *TusHandler) GetTusUploadReader(hash []byte, start int64) (io.ReadCloser, error) {
	ctx := context.Background()
	exists, upload := t.TusUploadExists(hash)

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

func (t *TusHandler) tusUploadTask(upload *models.TusUpload) error {
	ctx := context.Background()
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

	dbHash, err := hex.DecodeString(upload.Hash)

	if err != nil {
		t.logger.Error("Could not decode proof", zap.Error(err))
		return err
	}

	if !bytes.Equal(proof.Hash, dbHash) {
		t.logger.Error("Hashes do not match", zap.Any("upload", upload), zap.Any("proof", proof), zap.Any("dbHash", dbHash))
		return err
	}

	info, err := tusUpload.GetInfo(context.Background())
	if err != nil {
		t.logger.Error("Could not get tus info", zap.Error(err))
		return err
	}

	storageProtocol := GetStorageProtocol(t.protocol)

	uploadMeta, err := t.storage.UploadObject(ctx, storageProtocol, nil, &renter.MultiPartUploadParams{
		ReaderFactory: func(start uint, end uint) (io.ReadCloser, error) {
			rangeHeader := fmt.Sprintf("bytes=%d-%d", start, end)
			ctx = context.WithValue(ctx, "range", rangeHeader)
			return tusUpload.GetReader(ctx)
		},
		Bucket:   upload.Protocol,
		FileName: "/" + storageProtocol.EncodeFileName(dbHash),
		Size:     uint64(info.Size),
	}, proof)

	if err != nil {
		t.logger.Error("Could not upload file", zap.Error(err))
		return err
	}

	s3InfoId, _ := splitS3Ids(upload.UploadID)

	_, err = t.s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(t.config.GetString("core.storage.s3.bufferBucket")),
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

	err = t.accounts.PinByHash(dbHash, upload.UploaderID)
	if err != nil {
		t.logger.Error("Could not pin upload", zap.Error(err))
		return err
	}

	return nil
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

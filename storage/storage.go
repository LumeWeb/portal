package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/account"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/cron"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/renter"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"github.com/spf13/viper"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	"github.com/tus/tusd/v2/pkg/s3store"
	"go.sia.tech/renterd/api"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"gorm.io/gorm"
	"io"
	"lukechampine.com/blake3"
	"net/http"
	"strings"
	"time"
)

type TusPreUploadCreateCallback func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error)
type TusPreFinishResponseCallback func(hook tusd.HookEvent) (tusd.HTTPResponse, error)

type StorageServiceParams struct {
	fx.In
	Config   *viper.Viper
	Logger   *zap.Logger
	Db       *gorm.DB
	Accounts *account.AccountServiceDefault
	Cron     *cron.CronServiceDefault
}

var Module = fx.Module("storage",
	fx.Provide(
		NewStorageService,
	),
	fx.Invoke(func(s *StorageServiceDefault) error {
		return s.init()
	}),
)

type StorageServiceDefault struct {
	tus      *tusd.Handler
	tusStore tusd.DataStore
	s3Client *s3.Client
	config   *viper.Viper
	logger   *zap.Logger
	db       *gorm.DB
	accounts *account.AccountServiceDefault
	cron     *cron.CronServiceDefault
	renter   *renter.RenterDefault
}

func (s *StorageServiceDefault) Tus() *tusd.Handler {
	return s.tus
}

func (s *StorageServiceDefault) Start() error {
	return nil
}

func NewStorageService(params StorageServiceParams) *StorageServiceDefault {
	return &StorageServiceDefault{
		config:   params.Config,
		logger:   params.Logger,
		db:       params.Db,
		accounts: params.Accounts,
		cron:     params.Cron,
	}
}

func (s StorageServiceDefault) PutFileSmall(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error) {
	hash, err := s.GetHashSmall(file)
	hashStr, err := encoding.NewMultihash(s.getPrefixedHash(hash)).ToBase64Url()
	if err != nil {
		return nil, err
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	err = s.renter.CreateBucketIfNotExists(bucket)
	if err != nil {
		return nil, err
	}

	err = s.renter.UploadObject(context.Background(), file, bucket, hashStr)

	if err != nil {
		return nil, err
	}

	return hash[:], nil
}
func (s StorageServiceDefault) PutFile(file io.Reader, bucket string, hash []byte) error {
	hashStr, err := encoding.NewMultihash(s.getPrefixedHash(hash)).ToBase64Url()
	err = s.renter.CreateBucketIfNotExists(bucket)
	if err != nil {
		return err
	}

	err = s.renter.UploadObject(context.Background(), file, bucket, hashStr)
	if err != nil {
		return err
	}

	return nil
}

func (s *StorageServiceDefault) BuildUploadBufferTus(basePath string, preUploadCb TusPreUploadCreateCallback, preFinishCb TusPreFinishResponseCallback) (*tusd.Handler, tusd.DataStore, *s3.Client, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           s.config.GetString("core.storage.s3.endpoint"),
				SigningRegion: s.config.GetString("core.storage.s3.region"),
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.config.GetString("core.storage.s3.accessKey"),
			s.config.GetString("core.storage.s3.secretKey"),
			"",
		)),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, nil, nil, nil
	}

	s3Client := s3.NewFromConfig(cfg)

	store := s3store.New(s.config.GetString("core.storage.s3.bufferBucket"), s3Client)

	locker := NewMySQLLocker(s.db, s.logger)

	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	composer.UseLocker(locker)

	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:                basePath,
		StoreComposer:           composer,
		DisableDownload:         true,
		NotifyCompleteUploads:   true,
		NotifyTerminatedUploads: true,
		NotifyCreatedUploads:    true,
		RespectForwardedHeaders: true,
		PreUploadCreateCallback: preUploadCb,
	})

	return handler, store, s3Client, err
}

func (s *StorageServiceDefault) init() error {
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

		exists, _ := s.FileExists(decodedHash.HashBytes())

		if exists {
			return blankResp, blankChanges, errors.New("file already exists")
		}

		exists, _ = s.TusUploadExists(decodedHash.HashBytes())

		if exists {
			return blankResp, blankChanges, errors.New("file is already being uploaded")
		}

		return blankResp, blankChanges, nil
	}

	tus, store, s3client, err := s.BuildUploadBufferTus("/s5/upload/tus", preUpload, nil)

	if err != nil {
		return err
	}

	s.tus = tus
	s.tusStore = store
	s.s3Client = s3client

	s.cron.RegisterService(s)

	return nil
}
func (s *StorageServiceDefault) LoadInitialTasks(cron cron.CronService) error {
	return nil
}

func (s *StorageServiceDefault) FileExists(hash []byte) (bool, models.Upload) {
	hashStr := hex.EncodeToString(hash)

	var upload models.Upload
	result := s.db.Model(&models.Upload{}).Where(&models.Upload{Hash: hashStr}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (s *StorageServiceDefault) GetHashSmall(file io.ReadSeeker) ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	_, err := io.Copy(buf, file)
	if err != nil {
		return nil, err
	}

	hash := blake3.Sum256(buf.Bytes())

	return hash[:], nil
}
func (s *StorageServiceDefault) GetHash(file io.Reader) ([]byte, int64, error) {
	hasher := blake3.New(64, nil)

	totalBytes, err := io.Copy(hasher, file)

	if err != nil {
		return nil, 0, err
	}

	hash := hasher.Sum(nil)

	return hash[:32], totalBytes, nil
}

func (s *StorageServiceDefault) CreateUpload(hash []byte, mime string, uploaderID uint, uploaderIP string, size uint64, protocol string) (*models.Upload, error) {
	hashStr := hex.EncodeToString(hash)

	upload := &models.Upload{
		Hash:       hashStr,
		MimeType:   mime,
		UserID:     uploaderID,
		UploaderIP: uploaderIP,
		Protocol:   protocol,
		Size:       size,
	}

	result := s.db.Create(upload)

	if result.Error != nil {
		return nil, result.Error
	}

	return upload, nil
}
func (s *StorageServiceDefault) tusWorker() {

	for {
		select {
		case info := <-s.tus.CreatedUploads:
			hash, ok := info.Upload.MetaData["hash"]
			errorResponse := tusd.HTTPResponse{StatusCode: 400, Header: nil}
			if !ok {
				s.logger.Error("Missing hash in metadata")
				continue
			}

			uploaderID, ok := info.Context.Value(middleware.S5AuthUserIDKey).(uint64)
			if !ok {
				errorResponse.Body = "Missing user id in context"
				info.Upload.StopUpload(errorResponse)
				s.logger.Error("Missing user id in context")
				continue
			}

			uploaderIP := info.HTTPRequest.RemoteAddr

			decodedHash, err := encoding.MultihashFromBase64Url(hash)

			if err != nil {
				errorResponse.Body = "Could not decode hash"
				info.Upload.StopUpload(errorResponse)
				s.logger.Error("Could not decode hash", zap.Error(err))
				continue
			}

			_, err = s.CreateTusUpload(decodedHash.HashBytes(), info.Upload.ID, uint(uploaderID), uploaderIP, info.Context.Value("protocol").(string))
			if err != nil {
				errorResponse.Body = "Could not create tus upload"
				info.Upload.StopUpload(errorResponse)
				s.logger.Error("Could not create tus upload", zap.Error(err))
				continue
			}
		case info := <-s.tus.UploadProgress:
			err := s.TusUploadProgress(info.Upload.ID)
			if err != nil {
				s.logger.Error("Could not update tus upload", zap.Error(err))
				continue
			}
		case info := <-s.tus.TerminatedUploads:
			err := s.DeleteTusUpload(info.Upload.ID)
			if err != nil {
				s.logger.Error("Could not delete tus upload", zap.Error(err))
				continue
			}

		case info := <-s.tus.CompleteUploads:
			if !(!info.Upload.SizeIsDeferred && info.Upload.Offset == info.Upload.Size) {
				continue
			}
			err := s.TusUploadCompleted(info.Upload.ID)
			if err != nil {
				s.logger.Error("Could not complete tus upload", zap.Error(err))
				continue
			}
			err = s.ScheduleTusUpload(info.Upload.ID)
			if err != nil {
				s.logger.Error("Could not schedule tus upload", zap.Error(err))
				continue
			}

		}
	}
}

func (s *StorageServiceDefault) TusUploadExists(hash []byte) (bool, models.TusUpload) {
	hashStr := hex.EncodeToString(hash)

	var upload models.TusUpload
	result := s.db.Model(&models.TusUpload{}).Where(&models.TusUpload{Hash: hashStr}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (s *StorageServiceDefault) CreateTusUpload(hash []byte, uploadID string, uploaderID uint, uploaderIP string, protocol string) (*models.TusUpload, error) {
	hashStr := hex.EncodeToString(hash)

	upload := &models.TusUpload{
		Hash:       hashStr,
		UploadID:   uploadID,
		UploaderID: uploaderID,
		UploaderIP: uploaderIP,
		Uploader:   models.User{},
		Protocol:   protocol,
	}

	result := s.db.Create(upload)

	if result.Error != nil {
		return nil, result.Error
	}

	return upload, nil
}
func (s *StorageServiceDefault) TusUploadProgress(uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := s.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = s.db.Model(&models.TusUpload{}).Where(find).Update("updated_at", time.Now())

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s *StorageServiceDefault) TusUploadCompleted(uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := s.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = s.db.Model(&models.TusUpload{}).Where(find).Update("completed", true)

	return nil
}
func (s *StorageServiceDefault) DeleteTusUpload(uploadID string) error {
	result := s.db.Where(&models.TusUpload{UploadID: uploadID}).Delete(&models.TusUpload{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (s *StorageServiceDefault) ScheduleTusUpload(uploadID string) error {
	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := s.db.Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	task := s.cron.RetryableTask(cron.RetryableTaskParams{
		Name:     "tusUpload",
		Function: s.tusUploadTask,
		Args:     []interface{}{&upload},
		Attempt:  0,
		Limit:    0,
		After: func(jobID uuid.UUID, jobName string) {
			s.logger.Info("Job finished", zap.String("jobName", jobName), zap.String("uploadID", uploadID))
			err := s.DeleteTusUpload(uploadID)
			if err != nil {
				s.logger.Error("Error deleting tus upload", zap.Error(err))
			}
		},
	})

	_, err := s.cron.CreateJob(task)

	if err != nil {
		return err
	}
	return nil
}

func (s *StorageServiceDefault) tusUploadTask(upload *models.TusUpload) error {
	ctx := context.Background()
	tusUpload, err := s.tusStore.GetUpload(ctx, upload.UploadID)
	if err != nil {
		s.logger.Error("Could not get upload", zap.Error(err))
		return err
	}

	reader, err := tusUpload.GetReader(ctx)
	if err != nil {
		s.logger.Error("Could not get tus file", zap.Error(err))
		return err
	}

	hash, byteCount, err := s.GetHash(reader)

	if err != nil {
		s.logger.Error("Could not compute hash", zap.Error(err))
		return err
	}

	dbHash, err := hex.DecodeString(upload.Hash)

	if err != nil {
		s.logger.Error("Could not decode hash", zap.Error(err))
		return err
	}

	if !bytes.Equal(hash, dbHash) {
		s.logger.Error("Hashes do not match", zap.Any("upload", upload), zap.Any("hash", hash), zap.Any("dbHash", dbHash))
		return err
	}

	reader, err = tusUpload.GetReader(ctx)
	if err != nil {
		s.logger.Error("Could not get tus file", zap.Error(err))
		return err
	}

	var mimeBuf [512]byte

	_, err = reader.Read(mimeBuf[:])

	if err != nil {
		s.logger.Error("Could not read mime", zap.Error(err))
		return err
	}

	mimeType := http.DetectContentType(mimeBuf[:])

	upload.MimeType = mimeType

	if tx := s.db.Save(upload); tx.Error != nil {
		s.logger.Error("Could not update tus upload", zap.Error(tx.Error))
		return tx.Error
	}

	reader, err = tusUpload.GetReader(ctx)
	if err != nil {
		s.logger.Error("Could not get tus file", zap.Error(err))
		return err
	}

	err = s.PutFile(reader, upload.Protocol, dbHash)

	if err != nil {
		s.logger.Error("Could not upload file", zap.Error(err))
		return err
	}

	s3InfoId, _ := splitS3Ids(upload.UploadID)

	_, err = s.s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.config.GetString("core.storage.s3.bufferBucket")),
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
		s.logger.Error("Could not delete upload metadata", zap.Error(err))
		return err
	}

	newUpload, err := s.CreateUpload(dbHash, mimeType, upload.UploaderID, upload.UploaderIP, uint64(byteCount), upload.Protocol)
	if err != nil {
		s.logger.Error("Could not create upload", zap.Error(err))
		return err
	}

	err = s.accounts.PinByID(newUpload.ID, upload.UploaderID)
	if err != nil {
		s.logger.Error("Could not pin upload", zap.Error(err))
		return err
	}

	return nil
}

func (s *StorageServiceDefault) getPrefixedHash(hash []byte) []byte {
	return append([]byte{byte(types.HashTypeBlake3)}, hash...)
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

func (s *StorageServiceDefault) GetFile(hash []byte, start int64) (io.ReadCloser, int64, error) {
	if exists, tusUpload := s.TusUploadExists(hash); exists {
		if tusUpload.Completed {
			upload, err := s.tusStore.GetUpload(context.Background(), tusUpload.UploadID)
			if err != nil {
				return nil, 0, err
			}

			info, _ := upload.GetInfo(context.Background())

			ctx := context.Background()

			if start > 0 {
				endPosition := start + info.Size - 1
				rangeHeader := fmt.Sprintf("bytes=%d-%d", start, endPosition)
				ctx = context.WithValue(ctx, "range", rangeHeader)
			}

			reader, err := upload.GetReader(ctx)

			return reader, info.Size, err
		}
	}

	exists, upload := s.FileExists(hash)

	if !exists {
		return nil, 0, errors.New("file does not exist")
	}

	hashStr, err := encoding.NewMultihash(s.getPrefixedHash(hash)).ToBase64Url()
	if err != nil {
		return nil, 0, err
	}

	var partialRange api.DownloadRange

	if start > 0 {
		partialRange = api.DownloadRange{
			Offset: start,
			Length: int64(upload.Size) - start + 1,
			Size:   int64(upload.Size),
		}
	}

	object, err := s.renter.GetObject(context.Background(), upload.Protocol, hashStr, api.DownloadObjectOptions{
		Range: partialRange,
	})

	if err != nil {
		return nil, 0, err
	}

	return object.Content, int64(upload.Size), nil
}
func (s *StorageServiceDefault) NewFile(hash []byte) *FileImpl {
	return NewFile(hash, s)
}

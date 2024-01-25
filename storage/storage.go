package storage

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/api/middleware"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/go-co-op/gocron/v2"
	"github.com/google/uuid"
	tusd "github.com/tus/tusd/v2/pkg/handler"
	s3store "github.com/tus/tusd/v2/pkg/s3store"
	"go.sia.tech/renterd/api"
	busClient "go.sia.tech/renterd/bus/client"
	workerClient "go.sia.tech/renterd/worker/client"
	"go.uber.org/zap"
	"io"
	"lukechampine.com/blake3"
	"net/url"
	"strings"
	"time"
)

var (
	_ interfaces.StorageService = (*StorageServiceImpl)(nil)
)

type StorageServiceImpl struct {
	portal       interfaces.Portal
	busClient    *busClient.Client
	workerClient *workerClient.Client
	tus          *tusd.Handler
	tusStore     tusd.DataStore
	s3Client     *s3.Client
}

func (s *StorageServiceImpl) Tus() *tusd.Handler {
	return s.tus
}

func (s *StorageServiceImpl) Start() error {
	return nil
}

func (s *StorageServiceImpl) Portal() interfaces.Portal {
	return s.portal
}

func NewStorageService(portal interfaces.Portal) interfaces.StorageService {
	return &StorageServiceImpl{
		portal: portal,
	}
}

func (s StorageServiceImpl) PutFileSmall(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error) {
	hash, err := s.GetHashSmall(file)
	hashStr, err := encoding.NewMultihash(s.getPrefixedHash(hash)).ToBase64Url()
	if err != nil {
		return nil, err
	}

	_, err = file.Seek(0, io.SeekStart)
	if err != nil {
		return nil, err
	}

	err = s.createBucketIfNotExists(bucket)
	if err != nil {
		return nil, err
	}

	_, err = s.workerClient.UploadObject(context.Background(), file, bucket, hashStr, api.UploadObjectOptions{})

	if err != nil {
		return nil, err
	}

	return hash[:], nil
}
func (s StorageServiceImpl) PutFile(file io.Reader, bucket string, hash []byte) error {
	hashStr, err := encoding.NewMultihash(s.getPrefixedHash(hash)).ToBase64Url()
	err = s.createBucketIfNotExists(bucket)
	if err != nil {
		return err
	}

	_, err = s.workerClient.UploadObject(context.Background(), file, bucket, hashStr, api.UploadObjectOptions{})
	if err != nil {
		return err
	}

	return nil
}

func (s *StorageServiceImpl) BuildUploadBufferTus(basePath string, preUploadCb interfaces.TusPreUploadCreateCallback, preFinishCb interfaces.TusPreFinishResponseCallback) (*tusd.Handler, tusd.DataStore, *s3.Client, error) {
	customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		if service == s3.ServiceID {
			return aws.Endpoint{
				URL:           s.portal.Config().GetString("core.storage.s3.endpoint"),
				SigningRegion: s.portal.Config().GetString("core.storage.s3.region"),
			}, nil
		}
		return aws.Endpoint{}, &aws.EndpointNotFoundError{}
	})

	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			s.portal.Config().GetString("core.storage.s3.accessKey"),
			s.portal.Config().GetString("core.storage.s3.secretKey"),
			"",
		)),
		config.WithEndpointResolverWithOptions(customResolver),
	)
	if err != nil {
		return nil, nil, nil, nil
	}

	s3Client := s3.NewFromConfig(cfg)

	store := s3store.New(s.portal.Config().GetString("core.storage.s3.bufferBucket"), s3Client)

	locker := NewMySQLLocker(s)

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

func (s *StorageServiceImpl) Init() error {

	addr := s.portal.Config().GetString("core.sia.url")
	passwd := s.portal.Config().GetString("core.sia.key")

	addrURL, err := url.Parse(addr)

	if err != nil {
		return err
	}

	addrURL.Path = "/api/worker"

	s.workerClient = workerClient.New(addrURL.String(), passwd)

	addrURL.Path = "/api/bus"

	s.busClient = busClient.New(addrURL.String(), passwd)

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

	s.portal.CronService().RegisterService(s)

	go s.tusWorker()

	return nil
}
func (s *StorageServiceImpl) LoadInitialTasks(cron interfaces.CronService) error {
	return nil
}

func (s *StorageServiceImpl) createBucketIfNotExists(bucket string) error {

	_, err := s.busClient.Bucket(context.Background(), bucket)

	if err == nil {
		return nil
	}

	if err != nil {
		if !strings.Contains(err.Error(), "bucket not found") {
			return err
		}
	}

	err = s.busClient.CreateBucket(context.Background(), bucket, api.CreateBucketOptions{
		Policy: api.BucketPolicy{
			PublicReadAccess: false,
		},
	})
	if err != nil {
		return err
	}

	return nil
}

func (s *StorageServiceImpl) FileExists(hash []byte) (bool, models.Upload) {
	hashStr := hex.EncodeToString(hash)

	var upload models.Upload
	result := s.portal.Database().Model(&models.Upload{}).Where(&models.Upload{Hash: hashStr}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (s *StorageServiceImpl) GetHashSmall(file io.ReadSeeker) ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	_, err := io.Copy(buf, file)
	if err != nil {
		return nil, err
	}

	hash := blake3.Sum256(buf.Bytes())

	return hash[:], nil
}
func (s *StorageServiceImpl) GetHash(file io.Reader) ([]byte, int64, error) {
	hasher := blake3.New(64, nil)

	totalBytes, err := io.Copy(hasher, file)

	if err != nil {
		return nil, 0, err
	}

	hash := hasher.Sum(nil)

	return hash[:32], totalBytes, nil
}

func (s *StorageServiceImpl) CreateUpload(hash []byte, uploaderID uint, uploaderIP string, size uint64, protocol string) (*models.Upload, error) {
	hashStr := hex.EncodeToString(hash)

	upload := &models.Upload{
		Hash:       hashStr,
		UserID:     uploaderID,
		UploaderIP: uploaderIP,
		Protocol:   protocol,
		Size:       size,
	}

	result := s.portal.Database().Create(upload)

	if result.Error != nil {
		return nil, result.Error
	}

	return upload, nil
}
func (s *StorageServiceImpl) tusWorker() {

	for {
		select {
		case info := <-s.tus.CreatedUploads:
			hash, ok := info.Upload.MetaData["hash"]
			errorResponse := tusd.HTTPResponse{StatusCode: 400, Header: nil}
			if !ok {
				s.portal.Logger().Error("Missing hash in metadata")
				continue
			}

			uploaderID, ok := info.Context.Value(middleware.S5AuthUserIDKey).(uint64)
			if !ok {
				errorResponse.Body = "Missing user id in context"
				info.Upload.StopUpload(errorResponse)
				s.portal.Logger().Error("Missing user id in context")
				continue
			}

			uploaderIP := info.HTTPRequest.RemoteAddr

			decodedHash, err := encoding.MultihashFromBase64Url(hash)

			if err != nil {
				errorResponse.Body = "Could not decode hash"
				info.Upload.StopUpload(errorResponse)
				s.portal.Logger().Error("Could not decode hash", zap.Error(err))
				continue
			}

			_, err = s.CreateTusUpload(decodedHash.HashBytes(), info.Upload.ID, uint(uploaderID), uploaderIP, info.Context.Value("protocol").(string))
			if err != nil {
				errorResponse.Body = "Could not create tus upload"
				info.Upload.StopUpload(errorResponse)
				s.portal.Logger().Error("Could not create tus upload", zap.Error(err))
				continue
			}
		case info := <-s.tus.UploadProgress:
			err := s.TusUploadProgress(info.Upload.ID)
			if err != nil {
				s.portal.Logger().Error("Could not update tus upload", zap.Error(err))
				continue
			}
		case info := <-s.tus.TerminatedUploads:
			err := s.DeleteTusUpload(info.Upload.ID)
			if err != nil {
				s.portal.Logger().Error("Could not delete tus upload", zap.Error(err))
				continue
			}

		case info := <-s.tus.CompleteUploads:
			if !(!info.Upload.SizeIsDeferred && info.Upload.Offset == info.Upload.Size) {
				continue
			}
			err := s.TusUploadCompleted(info.Upload.ID)
			if err != nil {
				s.portal.Logger().Error("Could not complete tus upload", zap.Error(err))
				continue
			}
			err = s.ScheduleTusUpload(info.Upload.ID, 0)
			if err != nil {
				s.portal.Logger().Error("Could not schedule tus upload", zap.Error(err))
				continue
			}

		}
	}
}

func (s *StorageServiceImpl) TusUploadExists(hash []byte) (bool, models.TusUpload) {
	hashStr := hex.EncodeToString(hash)

	var upload models.TusUpload
	result := s.portal.Database().Model(&models.TusUpload{}).Where(&models.TusUpload{Hash: hashStr}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (s *StorageServiceImpl) CreateTusUpload(hash []byte, uploadID string, uploaderID uint, uploaderIP string, protocol string) (*models.TusUpload, error) {
	hashStr := hex.EncodeToString(hash)

	upload := &models.TusUpload{
		Hash:       hashStr,
		UploadID:   uploadID,
		UploaderID: uploaderID,
		UploaderIP: uploaderIP,
		Uploader:   models.User{},
		Protocol:   protocol,
	}

	result := s.portal.Database().Create(upload)

	if result.Error != nil {
		return nil, result.Error
	}

	return upload, nil
}
func (s *StorageServiceImpl) TusUploadProgress(uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := s.portal.Database().Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = s.portal.Database().Model(&models.TusUpload{}).Where(find).Update("updated_at", time.Now())

	if result.Error != nil {
		return result.Error
	}

	return nil
}
func (s *StorageServiceImpl) TusUploadCompleted(uploadID string) error {

	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := s.portal.Database().Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	result = s.portal.Database().Model(&models.TusUpload{}).Where(find).Update("completed", true)

	return nil
}
func (s *StorageServiceImpl) DeleteTusUpload(uploadID string) error {
	result := s.portal.Database().Where(&models.TusUpload{UploadID: uploadID}).Delete(&models.TusUpload{})

	if result.Error != nil {
		return result.Error
	}

	return nil
}

func (s *StorageServiceImpl) ScheduleTusUpload(uploadID string, attempt int) error {
	find := &models.TusUpload{UploadID: uploadID}

	var upload models.TusUpload
	result := s.portal.Database().Model(&models.TusUpload{}).Where(find).First(&upload)

	if result.RowsAffected == 0 {
		return errors.New("upload not found")
	}

	job, task := s.buildNewTusUploadTask(&upload)

	if attempt > 0 {
		job = gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(time.Now().Add(time.Duration(attempt) * time.Minute)))
	}

	_, err := s.portal.Cron().NewJob(job, task, gocron.WithEventListeners(gocron.AfterJobRunsWithError(func(jobID uuid.UUID, jobName string, err error) {
		s.portal.Logger().Error("Error running job", zap.Error(err))
		err = s.ScheduleTusUpload(uploadID, attempt+1)
		if err != nil {
			s.portal.Logger().Error("Error rescheduling job", zap.Error(err))
		}
	}),
		gocron.AfterJobRuns(func(jobID uuid.UUID, jobName string) {
			s.portal.Logger().Info("Job finished", zap.String("jobName", jobName), zap.String("uploadID", uploadID))
			err := s.DeleteTusUpload(uploadID)
			if err != nil {
				s.portal.Logger().Error("Error deleting tus upload", zap.Error(err))
			}
		})))

	if err != nil {
		return err
	}
	return nil
}

func (s *StorageServiceImpl) buildNewTusUploadTask(upload *models.TusUpload) (job gocron.JobDefinition, task gocron.Task) {
	job = gocron.OneTimeJob(gocron.OneTimeJobStartImmediately())

	task = gocron.NewTask(
		func(upload *models.TusUpload) error {
			ctx := context.Background()
			tusUpload, err := s.tusStore.GetUpload(ctx, upload.UploadID)
			if err != nil {
				s.portal.Logger().Error("Could not get upload", zap.Error(err))
				return err
			}

			reader, err := tusUpload.GetReader(ctx)
			if err != nil {
				s.portal.Logger().Error("Could not get tus file", zap.Error(err))
				return err
			}

			hash, byteCount, err := s.GetHash(reader)

			if err != nil {
				s.portal.Logger().Error("Could not compute hash", zap.Error(err))
				return err
			}

			dbHash, err := hex.DecodeString(upload.Hash)

			if err != nil {
				s.portal.Logger().Error("Could not decode hash", zap.Error(err))
				return err
			}

			if !bytes.Equal(hash, dbHash) {
				s.portal.Logger().Error("Hashes do not match", zap.Any("upload", upload), zap.Any("hash", hash), zap.Any("dbHash", dbHash))
				return err
			}

			reader, err = tusUpload.GetReader(ctx)
			if err != nil {
				s.portal.Logger().Error("Could not get tus file", zap.Error(err))
				return err
			}

			err = s.PutFile(reader, upload.Protocol, dbHash)

			if err != nil {
				s.portal.Logger().Error("Could not upload file", zap.Error(err))
				return err
			}

			s3InfoId, _ := splitS3Ids(upload.UploadID)

			_, err = s.s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
				Bucket: aws.String(s.portal.Config().GetString("core.storage.s3.bufferBucket")),
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
				s.portal.Logger().Error("Could not delete upload metadata", zap.Error(err))
				return err
			}

			newUpload, err := s.CreateUpload(dbHash, upload.UploaderID, upload.UploaderIP, uint64(byteCount), upload.Protocol)
			if err != nil {
				s.portal.Logger().Error("Could not create upload", zap.Error(err))
				return err
			}

			err = s.portal.Accounts().PinByID(newUpload.ID, upload.UploaderID)
			if err != nil {
				s.portal.Logger().Error("Could not pin upload", zap.Error(err))
				return err
			}

			return nil
		}, upload)

	return job, task
}

func (s *StorageServiceImpl) getPrefixedHash(hash []byte) []byte {
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

func (s *StorageServiceImpl) GetFile(hash []byte, start int64) (io.ReadCloser, int64, error) {
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

	object, err := s.workerClient.GetObject(context.Background(), upload.Protocol, hashStr, api.DownloadObjectOptions{
		Range: partialRange,
	})

	if err != nil {
		return nil, 0, err
	}

	return object.Content, int64(upload.Size), nil
}

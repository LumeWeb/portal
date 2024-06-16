package sync

import (
	"bytes"
	"encoding/hex"
	"errors"
	"github.com/go-co-op/gocron/v2"
	"go.lumeweb.com/portal/bao"
	"go.lumeweb.com/portal/core"
	"go.sia.tech/renterd/api"
	"go.sia.tech/renterd/object"
	"go.uber.org/zap"
	"io"
)

const CronTaskVerifyObjectName = "SyncVerifyObject"
const CronTaskUploadObjectName = "SyncUploadObject"
const CronTaskScanObjectsName = "SyncScanObjects"
const syncBucketName = "Sync"

type CronTaskVerifyObjectArgs struct {
	Hash       []byte     `json:"hash"`
	Object     []FileMeta `json:"object"`
	UploaderID uint64     `json:"uploader_id"`
}
type cronTaskUploadObjectArgs struct {
	Hash       []byte `json:"hash"`
	Protocol   string `json:"protocol"`
	Size       uint64 `json:"size"`
	UploaderID uint64 `json:"uploader_id"`
}

func CronTaskUploadObjectArgsFactory() any {
	return &cronTaskUploadObjectArgs{}
}

func CronTaskScanObjectsDefinition() gocron.JobDefinition {
	return gocron.DailyJob(1, gocron.NewAtTimes(gocron.NewAtTime(0, 0, 0)))
}

func getSyncProtocol(protocol string) (core.SyncProtocol, error) {
	proto, err := core.GetProtocol(protocol)
	if err != nil {
		return nil, err
	}

	syncProto, ok := proto.(core.SyncProtocol)

	if !ok {
		return nil, errors.New("protocol is not a Sync protocol")
	}

	return syncProto, nil
}

func encodeProtocolFileName(hash []byte, protocol string) (string, error) {
	syncProto, err := getSyncProtocol(protocol)
	if err != nil {
		return "", err
	}

	return syncProto.EncodeFileName(hash), nil
}

func CronTaskVerifyObject(input any, ctx core.Context) error {
	args, ok := input.(*CronTaskVerifyObjectArgs)
	if !ok {
		return errors.New("invalid arguments type")
	}
	logger := ctx.Logger()
	renter := ctx.Services().Renter()
	cron := ctx.Services().Cron()
	err := renter.CreateBucketIfNotExists(syncBucketName)
	if err != nil {
		return err
	}

	success := false

	var foundObject FileMeta

	for _, object_ := range args.Object {
		if !bytes.Equal(object_.Hash, args.Hash) {
			logger.Error("hash mismatch", zap.Binary("expected", args.Hash), zap.Binary("actual", object_.Hash))
			continue
		}

		fileName, err := encodeProtocolFileName(object_.Hash, object_.Protocol)
		if err != nil {
			logger.Error("failed to encode protocol file name", zap.Error(err))
			return err
		}

		err = renter.ImportObjectMetadata(ctx, syncBucketName, fileName, object.Object{
			Key:   object_.Key,
			Slabs: object_.Slabs,
		})

		if err != nil {
			logger.Error("failed to import object metadata", zap.Error(err))
			continue
		}

		objectRet, err := renter.GetObject(ctx, syncBucketName, fileName, api.DownloadObjectOptions{})
		if err != nil {
			return err
		}

		verifier := bao.NewVerifier(objectRet.Content, bao.Result{
			Hash:   object_.Hash,
			Proof:  object_.Proof,
			Length: uint(object_.Size),
		}, logger.Logger)

		_, err = io.Copy(io.Discard, verifier)
		if err != nil {
			logger.Error("failed to verify object", zap.Error(err))
			continue
		}

		success = true
		foundObject = object_
	}

	if success {
		err := cron.CreateJobIfNotExists(CronTaskUploadObjectName, cronTaskUploadObjectArgs{
			Hash:       args.Hash,
			Protocol:   foundObject.Protocol,
			Size:       foundObject.Size,
			UploaderID: args.UploaderID,
		}, []string{hex.EncodeToString(args.Hash)})
		if err != nil {
			return err
		}
	}

	return nil
}

type seekableSiaStream struct {
	rc    io.ReadCloser
	ctx   core.Context
	args  *cronTaskUploadObjectArgs
	pos   int64
	reset bool
	size  int64
}

func (r *seekableSiaStream) Read(p []byte) (n int, err error) {
	if r.reset {
		r.reset = false
		err := r.rc.Close()
		if err != nil {
			return 0, err
		}

		fileName, err := encodeProtocolFileName(r.args.Hash, r.args.Protocol)
		if err != nil {
			r.ctx.Logger().Error("failed to encode protocol file name", zap.Error(err))
			return 0, err
		}

		objectRet, err := r.ctx.Services().Renter().GetObject(r.ctx, syncBucketName, fileName, api.DownloadObjectOptions{})
		if err != nil {
			return 0, err
		}
		r.rc = objectRet.Content
		r.pos = 0
	}
	n, err = r.rc.Read(p)
	r.pos += int64(n)
	return n, err
}

func (r *seekableSiaStream) Seek(offset int64, whence int) (int64, error) {
	if offset == 0 && whence == io.SeekStart {
		r.reset = true
		return 0, nil
	}

	if offset == 0 && whence == io.SeekEnd {
		return r.size, nil
	}

	return 0, errors.New("seek not supported")
}

func (r *seekableSiaStream) Close() error {
	return r.rc.Close()
}

func CronTaskUploadObject(input any, ctx core.Context) error {
	args, ok := input.(*cronTaskUploadObjectArgs)
	if !ok {
		return errors.New("invalid arguments type")
	}

	logger := ctx.Logger()
	renter := ctx.Services().Renter()
	storage := ctx.Services().Storage()
	metadata := ctx.Services().Metadata()
	_sync := ctx.Services().Sync()

	fileName, err := encodeProtocolFileName(args.Hash, args.Protocol)
	if err != nil {
		logger.Error("failed to encode protocol file name", zap.Error(err))
		return err
	}

	objectRet, err := renter.GetObject(ctx, syncBucketName, fileName, api.DownloadObjectOptions{})
	if err != nil {
		return err
	}

	syncProtocol, err := getSyncProtocol(args.Protocol)
	if err != nil {
		logger.Error("failed to get Sync protocol", zap.Error(err))
		return err
	}

	storeProtocol := syncProtocol.StorageProtocol()

	wrapper := &seekableSiaStream{
		rc:   objectRet.Content,
		ctx:  ctx,
		args: args,
		size: objectRet.Size,
	}

	upload, err := storage.UploadObject(ctx, storeProtocol, wrapper, args.Size, nil, nil)

	if err != nil {
		return err
	}

	upload.UserID = uint(args.UploaderID)

	err = metadata.SaveUpload(ctx, *upload, true)
	if err != nil {
		return err
	}

	err = _sync.Update(*upload)

	if err != nil {
		return err
	}

	err = renter.DeleteObjectMetadata(ctx, syncBucketName, fileName)
	if err != nil {
		return err
	}

	return nil
}

func CronTaskScanObjects(_ any, ctx core.Context) error {
	logger := ctx.Logger()
	metadata := ctx.Services().Metadata()
	_sync := ctx.Services().Sync()
	uploads, err := metadata.GetAllUploads(ctx)
	if err != nil {
		return err
	}

	for _, upload := range uploads {
		err := _sync.Update(upload)
		if err != nil {
			logger.Error("failed to update upload", zap.Error(err))
		}
	}

	return nil
}

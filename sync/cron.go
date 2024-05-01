package sync

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"

	"github.com/LumeWeb/portal/bao"

	"go.sia.tech/renterd/api"

	"go.uber.org/zap"

	"go.sia.tech/renterd/object"

	"github.com/LumeWeb/portal/protocols/registry"
)

const cronTaskVerifyObjectName = "SyncVerifyObject"
const cronTaskUploadObjectName = "SyncUploadObject"
const syncBucketName = "sync"

type cronTaskVerifyObjectArgs struct {
	Hash       []byte     `json:"hash"`
	Object     []FileMeta `json:"object"`
	UploaderID uint64     `json:"uploader_id"`
}

func cronTaskVerifyObjectArgsFactory() any {
	return &cronTaskVerifyObjectArgs{}
}

type cronTaskUploadObjectArgs struct {
	Hash       []byte `json:"hash"`
	Protocol   string `json:"protocol"`
	Size       uint64 `json:"size"`
	UploaderID uint64 `json:"uploader_id"`
}

func cronTaskUploadObjectArgsFactory() any {
	return &cronTaskUploadObjectArgs{}
}

func getSyncProtocol(protocol string) (SyncProtocol, error) {
	proto := registry.GetProtocol(protocol)

	syncProto, ok := proto.(SyncProtocol)

	if !ok {
		return nil, errors.New("protocol is not a sync protocol")
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

func cronTaskVerifyObject(args *cronTaskVerifyObjectArgs, sync *SyncServiceDefault) error {
	ctx := context.Background()
	err := sync.renter.CreateBucketIfNotExists(syncBucketName)
	if err != nil {
		return err
	}

	success := false

	var foundObject FileMeta

	for _, object_ := range args.Object {
		if !bytes.Equal(object_.Hash, args.Hash) {
			sync.logger.Error("hash mismatch", zap.Binary("expected", args.Hash), zap.Binary("actual", object_.Hash))
			continue
		}

		fileName, err := encodeProtocolFileName(object_.Hash, object_.Protocol)
		if err != nil {
			sync.logger.Error("failed to encode protocol file name", zap.Error(err))
			return err
		}

		err = sync.renter.ImportObjectMetadata(ctx, syncBucketName, fileName, object.Object{
			Key:   object_.Key,
			Slabs: object_.Slabs,
		})

		if err != nil {
			sync.logger.Error("failed to import object metadata", zap.Error(err))
			continue
		}

		objectRet, err := sync.renter.GetObject(ctx, syncBucketName, fileName, api.DownloadObjectOptions{})
		if err != nil {
			return err
		}

		verifier := bao.NewVerifier(objectRet.Content, bao.Result{
			Hash:   object_.Hash,
			Proof:  object_.Proof,
			Length: uint(object_.Size),
		}, sync.logger)

		_, err = io.Copy(io.Discard, verifier)
		if err != nil {
			sync.logger.Error("failed to verify object", zap.Error(err))
			continue
		}

		success = true
		foundObject = object_
	}

	if success {
		err := sync.cron.CreateJobIfNotExists(cronTaskUploadObjectName, cronTaskUploadObjectArgs{
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
	sync  *SyncServiceDefault
	ctx   context.Context
	args  *cronTaskUploadObjectArgs
	pos   int64
	reset bool
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
			r.sync.logger.Error("failed to encode protocol file name", zap.Error(err))
			return 0, err
		}

		objectRet, err := r.sync.renter.GetObject(r.ctx, syncBucketName, fileName, api.DownloadObjectOptions{})
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
	return 0, errors.New("seek not supported")
}

func (r *seekableSiaStream) Close() error {
	return r.rc.Close()
}

func cronTaskUploadObject(args *cronTaskUploadObjectArgs, sync *SyncServiceDefault) error {
	ctx := context.Background()

	fileName, err := encodeProtocolFileName(args.Hash, args.Protocol)
	if err != nil {
		sync.logger.Error("failed to encode protocol file name", zap.Error(err))
		return err
	}

	objectRet, err := sync.renter.GetObject(ctx, syncBucketName, fileName, api.DownloadObjectOptions{})
	if err != nil {
		return err
	}

	syncProtocol, err := getSyncProtocol(args.Protocol)
	if err != nil {
		sync.logger.Error("failed to get sync protocol", zap.Error(err))
		return err
	}

	storeProtocol := syncProtocol.StorageProtocol()

	wrapper := &seekableSiaStream{
		rc:   objectRet.Content,
		sync: sync,
		ctx:  ctx,
		args: args,
	}

	upload, err := sync.storage.UploadObject(ctx, storeProtocol, wrapper, args.Size, nil, nil)

	if err != nil {
		return err
	}

	upload.UserID = uint(args.UploaderID)

	err = sync.metadata.SaveUpload(ctx, *upload, true)
	if err != nil {
		return err
	}

	err = sync.Update(*upload)

	if err != nil {
		return err
	}

	return nil
}

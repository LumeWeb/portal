package s5

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"time"

	"github.com/LumeWeb/portal/storage"

	"github.com/LumeWeb/portal/bao"

	"github.com/LumeWeb/portal/renter"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/LumeWeb/portal/db/models"

	tusd "github.com/tus/tusd/v2/pkg/handler"

	"github.com/LumeWeb/portal/metadata"
	"go.uber.org/zap"
)

const cronTaskTusUploadVerifyName = "TUSUploadVerify"
const cronTaskTusUploadProcessName = "TUSUploadProcess"
const cronTaskTusUploadCleanupName = "TUSUploadCleanup"

type cronTaskTusUploadVerifyArgs struct {
	Id string `json:"id"`
}

func cronTaskTusUploadVerifyArgsFactory() any {
	return &cronTaskTusUploadVerifyArgs{}
}

type cronTaskTusUploadProcessArgs struct {
	Id    string `json:"id"`
	Proof []byte `json:"proof"`
}

func cronTaskTusUploadProcessArgsFactory() any {
	return &cronTaskTusUploadProcessArgs{}
}

type cronTaskTusUploadCleanupArgs struct {
	Id       string `json:"id"`
	Protocol string `json:"protocol"`
	MimeType string `json:"mimeType"`
	Size     uint64 `json:"size"`
}

func cronTaskTusUploadCleanupArgsFactory() any {
	return &cronTaskTusUploadCleanupArgs{}
}

func getReader(ctx context.Context, upload tusd.Upload) (io.ReadCloser, error) {
	muReader, err := upload.GetReader(ctx)
	if err != nil {
		return nil, err
	}
	return muReader, nil
}

func closeReader(reader io.ReadCloser, tus *TusHandler) {
	err := reader.Close()
	if err != nil {
		tus.logger.Error("error closing reader", zap.Error(err))
	}
}

func cronTaskTusGetUpload(ctx context.Context, id string, tus *TusHandler) (*models.TusUpload, tusd.Upload, *tusd.FileInfo, error) {
	exists, upload := tus.UploadExists(ctx, id)

	if !exists {
		tus.logger.Error("Upload not found", zap.String("hash", hex.EncodeToString(upload.Hash)))
		return nil, nil, nil, metadata.ErrNotFound
	}

	tusUpload, err := tus.tusStore.GetUpload(ctx, upload.UploadID)
	if err != nil {
		tus.logger.Error("Could not get upload", zap.Error(err))
		return nil, nil, nil, err
	}

	info, err := tusUpload.GetInfo(ctx)
	if err != nil {
		tus.logger.Error("Could not get tus info", zap.Error(err))
		return nil, nil, nil, err
	}

	return &upload, tusUpload, &info, nil
}

func cronTaskTusUploadVerify(args *cronTaskTusUploadVerifyArgs, tus *TusHandler) error {
	ctx := context.Background()

	upload, tusUpload, info, err := cronTaskTusGetUpload(ctx, args.Id, tus)
	if err != nil {
		return err
	}

	reader, err := getReader(ctx, tusUpload)
	if err != nil {
		tus.logger.Error("Could not get tus file", zap.Error(err))
		return err
	}

	defer closeReader(reader, tus)

	proof, err := tus.storage.HashObject(ctx, reader, uint64(info.Size))

	if err != nil {
		tus.logger.Error("Could not compute proof", zap.Error(err))
		return err
	}

	if !bytes.Equal(proof.Hash, upload.Hash) {
		tus.logger.Error("Hashes do not match", zap.Any("upload", upload), zap.Any("dbHash", hex.EncodeToString(upload.Hash)))
		return err
	}

	err = tus.cron.CreateJob(cronTaskTusUploadProcessName, cronTaskTusUploadProcessArgs{
		Id:    args.Id,
		Proof: proof.Proof,
	}, []string{upload.UploadID})
	if err != nil {
		return err
	}

	return nil
}

func cronTaskTusUploadProcess(args *cronTaskTusUploadProcessArgs, tus *TusHandler) error {
	ctx := context.Background()

	upload, tusUpload, info, err := cronTaskTusGetUpload(ctx, args.Id, tus)
	if err != nil {
		return err
	}

	var uploadMeta metadata.UploadMetadata

	doUpload := true

	pinned, err := tus.accounts.UploadPinnedByUser(upload.Hash, upload.UploaderID)
	if err != nil {
		return err
	}

	if !pinned {
		status, err := tus.storage.UploadStatus(ctx, tus.storageProtocol, tus.storageProtocol.EncodeFileName(upload.Hash))
		if err != nil {
			return err
		}

		if status == storage.UploadStatusActive {
			doUpload = false

			err = waitForUploadCompletion(ctx, tus, upload.Hash)
			if err != nil {
				return err
			}
		}
	} else {
		doUpload = false
	}

	if doUpload {
		meta, err := tus.storage.UploadObject(ctx, tus.storageProtocol, nil, 0, &renter.MultiPartUploadParams{
			ReaderFactory: func(start uint, end uint) (io.ReadCloser, error) {
				rangeHeader := "bytes=%d-"
				if end != 0 {
					rangeHeader += "%d"
					rangeHeader = fmt.Sprintf("bytes=%d-%d", start, end)
				} else {
					rangeHeader = fmt.Sprintf("bytes=%d-", start)
				}
				ctx = context.WithValue(ctx, "range", rangeHeader)
				return tusUpload.GetReader(ctx)
			},
			Bucket:   upload.Protocol,
			FileName: tus.storageProtocol.EncodeFileName(upload.Hash),
			Size:     uint64(info.Size),
		}, &bao.Result{
			Hash:   upload.Hash,
			Proof:  args.Proof,
			Length: uint(info.Size),
		})

		if err != nil {
			tus.logger.Error("Could not upload file", zap.Error(err))
			return err
		}

		uploadMeta = *meta
	} else {
		meta, err := tus.metadata.GetUpload(ctx, upload.Hash)
		if err != nil {
			return err
		}

		uploadMeta = meta
	}

	err = tus.cron.CreateJob(cronTaskTusUploadCleanupName, cronTaskTusUploadCleanupArgs{
		Protocol: uploadMeta.Protocol,
		Id:       args.Id,
		MimeType: uploadMeta.MimeType,
		Size:     uploadMeta.Size,
	}, []string{upload.UploadID})
	if err != nil {
		return err
	}

	return nil
}

func cronTaskTusUploadCleanup(args *cronTaskTusUploadCleanupArgs, tus *TusHandler) error {
	ctx := context.Background()

	upload, _, _, err := cronTaskTusGetUpload(ctx, args.Id, tus)
	if err != nil {
		return err
	}

	s3InfoId, _ := splitS3Ids(upload.UploadID)

	_, err = tus.s3Client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(tus.config.Config().Core.Storage.S3.BufferBucket),
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
		tus.logger.Error("Could not delete upload metadata", zap.Error(err))
		return err
	}

	uploadMeta := metadata.UploadMetadata{
		Hash:     upload.Hash,
		MimeType: args.MimeType,
		Protocol: args.Protocol,
		Size:     args.Size,
	}

	uploadMeta.UserID = upload.UploaderID
	uploadMeta.UploaderIP = upload.UploaderIP

	err = tus.metadata.SaveUpload(ctx, uploadMeta, true)
	if err != nil {
		tus.logger.Error("Could not create upload", zap.Error(err))
		return err
	}

	err = tus.accounts.PinByHash(upload.Hash, upload.UploaderID)
	if err != nil {
		tus.logger.Error("Could not pin upload", zap.Error(err))
		return err
	}

	err = tus.DeleteUpload(ctx, upload.UploadID)
	if err != nil {
		tus.logger.Error("Error deleting tus upload", zap.Error(err))
		return err
	}

	return nil
}
func waitForUploadCompletion(ctx context.Context, tus *TusHandler, hash []byte) error {
	for {
		status, err := tus.storage.UploadStatus(ctx, tus.storageProtocol, tus.storageProtocol.EncodeFileName(hash))
		if err != nil {
			return err
		}
		if status != storage.UploadStatusActive {
			break
		}
		time.Sleep(5 * time.Second)
	}
	return nil
}

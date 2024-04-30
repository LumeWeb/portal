package s5

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/LumeWeb/portal/bao"
	"github.com/LumeWeb/portal/renter"
	"github.com/LumeWeb/portal/storage"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/LumeWeb/portal/protocols/s5"

	"github.com/LumeWeb/portal/metadata"

	"github.com/ddo/rq"

	"github.com/LumeWeb/libs5-go/encoding"
	"github.com/LumeWeb/portal/db/models"
	"go.uber.org/zap"
)

const totalPinImportStages = 3
const cronTaskPinImportValidateName = "PinImportValidate"

type cronTaskPinImportValidateArgs struct {
	Cid      string
	Url      string
	ProofUrl string
	UserId   uint
}

func cronTaskPinImportValidateArgsFactory() any {
	return &cronTaskPinImportValidateArgs{}
}

const cronTaskPinImportProcessSmallFileName = "PinImportVerify"

type cronTaskPinImportProcessSmallFileArgs struct {
	Cid      string
	Url      string
	ProofUrl string
	UserId   uint
}

func cronTaskPinImportProcessSmallFileArgsFactory() any {
	return &cronTaskPinImportProcessSmallFileArgs{}
}

const cronTaskPinImportProcessLargeFileName = "PinImportProcessLarge"

type cronTaskPinImportProcessLargeFileArgs struct {
	Cid      string
	Url      string
	ProofUrl string
	UserId   uint
}

func cronTaskPinImportProcessLargeFileArgsFactory() any {
	return &cronTaskPinImportProcessLargeFileArgs{}
}

func pinImportCloseBody(body io.ReadCloser, api *S5API) {
	if err := body.Close(); err != nil {
		api.logger.Error("error closing response body", zap.Error(err))
	}
}

func pinImportFetchAndProcess(fetchUrl string, progressStage int, api *S5API, cid *encoding.CID) ([]byte, error) {
	ctx := context.Background()
	req, err := rq.Get(fetchUrl).ParseRequest()
	if err != nil {
		api.logger.Error("error parsing request", zap.Error(err))
		return nil, err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		api.logger.Error("error executing request", zap.Error(err))
		return nil, err
	}

	defer pinImportCloseBody(res.Body, api)

	if res.StatusCode != http.StatusOK {
		errMsg := "error fetching URL: " + fetchUrl
		api.logger.Error(errMsg, zap.String("status", res.Status))
		return nil, fmt.Errorf(errMsg+" with status: %s", res.Status)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		api.logger.Error("error reading response body", zap.Error(err))
		return nil, err
	}

	err = api._import.UpdateProgress(ctx, cid.Hash.HashBytes(), progressStage, totalPinImportStages)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func pinImportSaveAndPin(upload *metadata.UploadMetadata, api *S5API, cid *encoding.CID, userId uint) error {
	ctx := context.Background()

	err := api._import.UpdateProgress(ctx, cid.Hash.HashBytes(), 3, totalPinImportStages)
	if err != nil {
		return err
	}

	upload.UserID = userId
	if err := api.metadata.SaveUpload(ctx, *upload, true); err != nil {
		return err
	}

	if err := api.accounts.PinByHash(upload.Hash, userId); err != nil {
		return err
	}

	err = api._import.DeleteImport(ctx, upload.Hash)
	if err != nil {
		return err
	}

	return nil
}

func cronTaskPinImportValidate(args *cronTaskPinImportValidateArgs, api *S5API) error {
	ctx := context.Background()

	// Parse CID early to avoid unnecessary operations if it fails.
	parsedCid, err := encoding.CIDFromString(args.Cid)
	if err != nil {
		api.logger.Error("error parsing cid", zap.Error(err))
		return err
	}

	err = api._import.UpdateStatus(ctx, parsedCid.Hash.HashBytes(), models.ImportStatusProcessing)
	if err != nil {
		return err
	}

	if parsedCid.Size <= api.config.Config().Core.PostUploadLimit {
		err = api.cron.CreateJobIfNotExists(cronTaskPinImportProcessSmallFileName, cronTaskPinImportProcessSmallFileArgs{
			Cid:      args.Cid,
			Url:      args.Url,
			ProofUrl: args.ProofUrl,
			UserId:   args.UserId,
		}, []string{args.Cid})
		if err != nil {
			return err
		}
	}

	err = api.cron.CreateJobIfNotExists(cronTaskPinImportProcessLargeFileName, cronTaskPinImportProcessLargeFileArgs{
		Cid:      args.Cid,
		Url:      args.Url,
		ProofUrl: args.ProofUrl,
		UserId:   args.UserId,
	}, []string{args.Cid})
	if err != nil {
		return err
	}

	return nil
}

func cronTaskPinImportProcessSmallFile(args *cronTaskPinImportProcessSmallFileArgs, api *S5API) error {
	ctx := context.Background()

	parsedCid, err := encoding.CIDFromString(args.Cid)
	if err != nil {
		api.logger.Error("error parsing cid", zap.Error(err))
		return err
	}

	fileData, err := pinImportFetchAndProcess(args.Url, 1, api, parsedCid)
	if err != nil {
		return err // Error logged in fetchAndProcess
	}

	hash, err := api.storage.HashObject(ctx, bytes.NewReader(fileData), uint64(len(fileData)))
	if err != nil {
		api.logger.Error("error hashing object", zap.Error(err))
		return err
	}

	if !bytes.Equal(hash.Hash, parsedCid.Hash.HashBytes()) {
		return fmt.Errorf("hash mismatch")
	}

	err = api._import.UpdateProgress(ctx, parsedCid.Hash.HashBytes(), 2, totalPinImportStages)
	if err != nil {
		return err
	}

	upload, err := api.storage.UploadObject(ctx, s5.GetStorageProtocol(api.protocol), bytes.NewReader(fileData), parsedCid.Size, nil, hash)
	if err != nil {
		return err
	}

	err = pinImportSaveAndPin(upload, api, parsedCid, args.UserId)
	if err != nil {
		return err
	}

	err = api.sync.Update(*upload)

	if err != nil {
		return err
	}

	return nil
}

func cronTaskPinImportProcessLargeFile(args *cronTaskPinImportProcessLargeFileArgs, api *S5API) error {
	ctx := context.Background()

	parsedCid, err := encoding.CIDFromString(args.Cid)
	if err != nil {
		api.logger.Error("error parsing cid", zap.Error(err))
		return err
	}

	// Fetch proof.
	proof, err := pinImportFetchAndProcess(args.ProofUrl, 1, api, parsedCid)
	if err != nil {
		return err
	}

	baoProof := bao.Result{
		Hash:   parsedCid.Hash.HashBytes(),
		Proof:  proof,
		Length: uint(parsedCid.Size),
	}

	client, err := api.storage.S3Client(ctx)
	if err != nil {
		api.logger.Error("error getting s3 client", zap.Error(err))
		return err
	}

	req, err := rq.Get(args.Url).ParseRequest()
	if err != nil {
		api.logger.Error("error parsing request", zap.Error(err))
		return err
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		api.logger.Error("error executing request", zap.Error(err))
		return err
	}
	defer pinImportCloseBody(res.Body, api)

	verifier := bao.NewVerifier(res.Body, baoProof, api.logger)
	defer pinImportCloseBody(verifier, api)

	if parsedCid.Size < storage.S3_MULTIPART_MIN_PART_SIZE {
		_, err = client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(api.config.Config().Core.Storage.S3.BufferBucket),
			Key:           aws.String(args.Cid),
			Body:          verifier,
			ContentLength: aws.Int64(int64(parsedCid.Size)),
		})
		if err != nil {
			api.logger.Error("error uploading object", zap.Error(err))
			return err
		}
	} else {
		err := api.storage.S3MultipartUpload(ctx, verifier, api.config.Config().Core.Storage.S3.BufferBucket, args.Cid, parsedCid.Size)
		if err != nil {
			api.logger.Error("error uploading object", zap.Error(err))
			return err
		}
	}

	err = api._import.UpdateProgress(ctx, parsedCid.Hash.HashBytes(), 2, totalPinImportStages)
	if err != nil {
		return err
	}

	upload, err := api.storage.UploadObject(ctx, s5.GetStorageProtocol(api.protocol), nil, 0, &renter.MultiPartUploadParams{
		ReaderFactory: func(start uint, end uint) (io.ReadCloser, error) {
			rangeHeader := "bytes=%d-"
			if end != 0 {
				rangeHeader += "%d"
				rangeHeader = fmt.Sprintf("bytes=%d-%d", start, end)
			} else {
				rangeHeader = fmt.Sprintf("bytes=%d-", start)
			}
			object, err := client.GetObject(ctx, &s3.GetObjectInput{
				Bucket: aws.String(api.config.Config().Core.Storage.S3.BufferBucket),
				Key:    aws.String(args.Cid),
				Range:  aws.String(rangeHeader),
			})

			if err != nil {
				return nil, err
			}

			return object.Body, nil
		},
		Bucket:          api.config.Config().Core.Storage.S3.BufferBucket,
		FileName:        s5.GetStorageProtocol(api.protocol).EncodeFileName(parsedCid.Hash.HashBytes()),
		Size:            parsedCid.Size,
		UploadIDHandler: nil,
	}, &baoProof)

	if err != nil {
		api.logger.Error("error uploading object", zap.Error(err))
		return err
	}

	err = pinImportSaveAndPin(upload, api, parsedCid, args.UserId)
	if err != nil {
		return err
	}

	err = api.sync.Update(*upload)

	if err != nil {
		return err
	}

	return nil
}

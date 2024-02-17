package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"go.uber.org/fx"

	"go.sia.tech/renterd/api"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"go.uber.org/zap"

	"git.lumeweb.com/LumeWeb/portal/bao"

	"github.com/spf13/viper"
	"gorm.io/gorm"

	"git.lumeweb.com/LumeWeb/portal/renter"
)

const PROOF_EXTENSION = ".obao"

var _ StorageService = (*StorageServiceDefault)(nil)

type FileNameEncoderFunc func([]byte) string

type StorageProtocol interface {
	Name() string
	EncodeFileName([]byte) string
}

var Module = fx.Module("storage",
	fx.Provide(
		fx.Annotate(
			NewStorageService,
			fx.As(new(StorageService)),
		),
	),
)

type StorageService interface {
	UploadObject(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, muParams *renter.MultiPartUploadParams, proof *bao.Result) (*metadata.UploadMetadata, error)
	UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof *bao.Result) error
	HashObject(ctx context.Context, data io.Reader) (*bao.Result, error)
	DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash []byte, start int64) (io.ReadCloser, error)
	DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) (io.ReadCloser, error)
	DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash []byte) error
	DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) error
}

type StorageServiceDefault struct {
	config   *viper.Viper
	db       *gorm.DB
	renter   *renter.RenterDefault
	logger   *zap.Logger
	metadata metadata.MetadataService
}

type StorageServiceParams struct {
	Config   *viper.Viper
	Db       *gorm.DB
	Renter   *renter.RenterDefault
	Logger   *zap.Logger
	metadata metadata.MetadataService
}

func NewStorageService(params StorageServiceParams) *StorageServiceDefault {
	return &StorageServiceDefault{
		config: params.Config,
		db:     params.Db,
		renter: params.Renter,
	}
}

func (s StorageServiceDefault) UploadObject(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, muParams *renter.MultiPartUploadParams, proof *bao.Result) (*metadata.UploadMetadata, error) {
	readers := make([]io.ReadCloser, 0)
	defer func() {
		for _, reader := range readers {
			err := reader.Close()
			if err != nil {
				s.logger.Error("error closing reader", zap.Error(err))
			}
		}
	}()

	getReader := func() (io.Reader, error) {
		if muParams != nil {
			muReader, err := muParams.ReaderFactory(0, uint(muParams.Size))
			if err != nil {
				return nil, err
			}

			found := false
			for _, reader := range readers {
				if reader == muReader {
					found = true
					break
				}
			}

			if !found {
				readers = append(readers, muReader)
			}
		}

		_, err := data.Seek(0, io.SeekStart)
		if err != nil {
			return nil, err
		}

		return data, nil
	}

	reader, err := getReader()
	if err != nil {
		return nil, err
	}

	if proof == nil {
		hashResult, err := s.HashObject(ctx, reader)
		if err != nil {
			return nil, err
		}

		reader, err = getReader()
		if err != nil {
			return nil, err
		}

		proof = hashResult
	}

	mimeBytes := make([]byte, 512)
	_, err = io.ReadFull(data, mimeBytes)
	if err != nil {
		return nil, err
	}

	reader, err = getReader()
	if err != nil {
		return nil, err
	}

	mimeType := http.DetectContentType(mimeBytes)

	protocolName := protocol.Name()

	err = s.renter.CreateBucketIfNotExists(protocolName)
	if err != nil {
		return nil, err
	}

	filename := protocol.EncodeFileName(proof.Hash)

	err = s.UploadObjectProof(ctx, protocol, nil, proof)

	if err != nil {
		return nil, err
	}

	if muParams != nil {
		muParams.FileName = filename
		muParams.Bucket = protocolName

		err = s.renter.UploadObjectMultipart(ctx, muParams)
		if err != nil {
			return nil, err
		}

		return &metadata.UploadMetadata{
			Protocol: protocolName,
			Hash:     proof.Hash,
			MimeType: mimeType,
			Size:     uint64(proof.Length),
		}, nil
	}

	err = s.renter.UploadObject(ctx, reader, protocolName, filename)
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (s StorageServiceDefault) UploadObjectProof(ctx context.Context, protocol StorageProtocol, data io.ReadSeeker, proof *bao.Result) error {
	if proof == nil {
		hashResult, err := s.HashObject(ctx, data)
		if err != nil {
			return err
		}

		proof = hashResult
	}

	protocolName := protocol.Name()

	err := s.renter.CreateBucketIfNotExists(protocolName)

	if err != nil {
		return err
	}

	return s.renter.UploadObject(ctx, bytes.NewReader(proof.Proof), protocolName, s.getProofPath(protocol, proof.Hash))
}

func (s StorageServiceDefault) HashObject(ctx context.Context, data io.Reader) (*bao.Result, error) {
	result, err := bao.Hash(data)

	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s StorageServiceDefault) DownloadObject(ctx context.Context, protocol StorageProtocol, objectHash []byte, start int64) (io.ReadCloser, error) {
	var partialRange api.DownloadRange

	upload, err := s.metadata.GetUpload(ctx, objectHash)
	if err != nil {
		return nil, err
	}

	if start > 0 {
		partialRange = api.DownloadRange{
			Offset: start,
			Length: int64(upload.Size) - start + 1,
			Size:   int64(upload.Size),
		}
	}

	object, err := s.renter.GetObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash), api.DownloadObjectOptions{Range: partialRange})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DownloadObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) (io.ReadCloser, error) {
	object, err := s.renter.GetObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash)+".bao", api.DownloadObjectOptions{})
	if err != nil {
		return nil, err
	}

	return object.Content, nil
}

func (s StorageServiceDefault) DeleteObject(ctx context.Context, protocol StorageProtocol, objectHash []byte) error {
	err := s.renter.DeleteObject(ctx, protocol.Name(), protocol.EncodeFileName(objectHash))
	if err != nil {
		return err
	}

	return nil
}

func (s StorageServiceDefault) DeleteObjectProof(ctx context.Context, protocol StorageProtocol, objectHash []byte) error {
	err := s.renter.DeleteObject(ctx, protocol.Name(), s.getProofPath(protocol, objectHash))
	if err != nil {
		return err
	}

	return nil
}

func (s StorageServiceDefault) getProofPath(protocol StorageProtocol, objectHash []byte) string {
	return fmt.Sprintf("%s/%s.%s", protocol.Name(), protocol.EncodeFileName(objectHash), PROOF_EXTENSION)
}

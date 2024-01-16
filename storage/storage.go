package storage

import (
	"bytes"
	"encoding/hex"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/go-resty/resty/v2"
	"go.uber.org/zap"
	"io"
	"lukechampine.com/blake3"
)

var (
	_ interfaces.StorageService = (*StorageServiceImpl)(nil)
)

type StorageServiceImpl struct {
	portal  interfaces.Portal
	httpApi *resty.Client
}

func NewStorageService(portal interfaces.Portal) interfaces.StorageService {
	return &StorageServiceImpl{
		portal:  portal,
		httpApi: nil,
	}
}

func (s StorageServiceImpl) PutFile(file io.ReadSeeker, bucket string, generateProof bool) ([]byte, error) {

	hash, err := s.GetHash(file)
	hashStr, err := encoding.NewMultihash(hash[:]).ToBase64Url()
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

	resp, err := s.httpApi.R().
		SetPathParam("path", hashStr).
		SetFormData(map[string]string{
			"bucket": bucket,
		}).
		SetBody(file).Put("/api/worker/objects/{path}")
	if err != nil {
		return nil, err
	}

	s.portal.Logger().Info("resp", zap.Any("resp", resp.String()))

	if resp.IsError() && resp.Error() != nil {
		return nil, resp.Error().(error)
	}

	return hash[:], nil
}

func (s *StorageServiceImpl) Init() {
	client := resty.New()
	client.SetDisableWarn(true)

	client.SetBaseURL(s.portal.Config().GetString("core.sia.url"))
	client.SetBasicAuth("", s.portal.Config().GetString("core.sia.key"))

	s.httpApi = client
}
func (s *StorageServiceImpl) createBucketIfNotExists(bucket string) error {
	resp, err := s.httpApi.R().
		SetPathParam("bucket", bucket).
		Get("/api/bus/bucket/{bucket}")

	if err != nil {
		return err
	}

	if resp.StatusCode() != 404 {
		if resp.IsError() && resp.Error() != nil {
			return resp.Error().(error)
		}
	} else {
		resp, err := s.httpApi.R().
			SetBody(map[string]string{
				"name": bucket,
			}).
			Post("/api/bus/buckets")
		if err != nil {
			return err
		}

		if resp.IsError() && resp.Error() != nil {
			return resp.Error().(error)
		}
	}

	return nil
}

func (s *StorageServiceImpl) FileExists(hash []byte) (bool, models.Upload) {
	hashStr := hex.EncodeToString(hash)

	var upload models.Upload
	result := s.portal.Database().Model(&models.Upload{}).Where(&models.Upload{Hash: hashStr}).First(&upload)

	return result.RowsAffected > 0, upload
}

func (s *StorageServiceImpl) GetHash(file io.ReadSeeker) ([]byte, error) {
	buf := bytes.NewBuffer(nil)

	_, err := io.Copy(buf, file)
	if err != nil {
		return nil, err
	}

	hash := blake3.Sum256(buf.Bytes())

	return hash[:], nil
}

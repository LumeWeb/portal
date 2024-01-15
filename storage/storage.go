package storage

import (
	"bytes"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
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
	buf := bytes.NewBuffer(nil)

	_, err := io.Copy(buf, file)
	if err != nil {
		return nil, err
	}

	hash := blake3.Sum512(buf.Bytes())
	hashStr, err := encoding.NewMultihash(hash[:]).ToBase64Url()
	if err != nil {
		return nil, err
	}

	buf.Reset()

	resp, err := s.httpApi.R().
		SetPathParam("path", hashStr).
		SetFormData(map[string]string{
			"bucket": bucket,
		}).
		SetBody(buf).Put("/api/worker/object/{path}")
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

	client.SetBaseURL(s.portal.Config().GetString("core.sia.url"))
	client.SetBasicAuth("", s.portal.Config().GetString("core.sia.key"))

	s.httpApi = client
}

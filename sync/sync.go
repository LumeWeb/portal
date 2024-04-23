package sync

import (
	"context"
	"errors"

	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/metadata"
	"github.com/LumeWeb/portal/protocols/registry"
	"github.com/LumeWeb/portal/renter"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

type SyncServiceDefault struct {
	config *config.Manager
	renter *renter.RenterDefault
	logger *zap.Logger
}

type SyncServiceParams struct {
	fx.In
	Config *config.Manager
	Renter *renter.RenterDefault
	Logger *zap.Logger
}

type SyncProtocol interface {
	Name() string
	EncodeFileName([]byte) string
}

var Module = fx.Module("sync",
	fx.Options(
		fx.Provide(NewSyncServiceDefault),
	),
)

func NewSyncServiceDefault(params SyncServiceParams) *SyncServiceDefault {
	return &SyncServiceDefault{
		config: params.Config,
		renter: params.Renter,
		logger: params.Logger,
	}
}

func (s *SyncServiceDefault) Update(upload metadata.UploadMetadata) error {
	proto := registry.GetProtocol(upload.Protocol)
	ctx := context.Background()

	if proto == nil {
		return errors.New("protocol not found")
	}

	syncProto, ok := proto.(SyncProtocol)
	if !ok {
		return errors.New("protocol is not a sync protocol")
	}

	fileName := syncProto.EncodeFileName(upload.Hash)

	object, err := s.renter.GetObjectMetadata(ctx, upload.Protocol, fileName)
	if err != nil {
		return err
	}

	_ = object

	// TODO: Implement sending the metadata to hypercore

	return nil
}

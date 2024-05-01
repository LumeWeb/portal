package sync

import (
	"context"
	"crypto/ed25519"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"

	"github.com/LumeWeb/portal/cron"

	"github.com/LumeWeb/portal/storage"

	"github.com/hashicorp/go-plugin"

	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/metadata"
	"github.com/LumeWeb/portal/protocols/registry"
	"github.com/LumeWeb/portal/renter"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var _ cron.CronableService = (*SyncServiceDefault)(nil)

//go:generate bash -c "cd proto && buf generate"

type SyncServiceDefault struct {
	config     *config.Manager
	renter     *renter.RenterDefault
	metadata   metadata.MetadataService
	storage    storage.StorageService
	cron       *cron.CronServiceDefault
	logger     *zap.Logger
	grpcClient *plugin.Client
	grpcPlugin sync
	identity   ed25519.PrivateKey
	logKey     []byte
}

type SyncServiceParams struct {
	fx.In
	Config   *config.Manager
	Renter   *renter.RenterDefault
	Metadata metadata.MetadataService
	Storage  storage.StorageService
	Cron     *cron.CronServiceDefault
	Logger   *zap.Logger
	Identity ed25519.PrivateKey
}

type SyncProtocol interface {
	Name() string
	EncodeFileName([]byte) string
	ValidIdentifier(string) bool
	StorageProtocol() storage.StorageProtocol
}

var Module = fx.Module("sync",
	fx.Options(
		fx.Provide(NewSyncService),
		fx.Invoke(func(lifecycle fx.Lifecycle, service *SyncServiceDefault) error {
			lifecycle.Append(fx.Hook{
				OnStart: func(context.Context) error {
					return service.init()
				},
				OnStop: func(context.Context) error {
					return service.stop()
				},
			})
			return nil
		}),
	),
)

func NewSyncService(params SyncServiceParams) *SyncServiceDefault {
	return &SyncServiceDefault{
		config:   params.Config,
		renter:   params.Renter,
		metadata: params.Metadata,
		storage:  params.Storage,
		cron:     params.Cron,
		logger:   params.Logger,
		identity: params.Identity,
	}
}

func (s *SyncServiceDefault) RegisterTasks(crn cron.CronService) error {
	crn.RegisterTask(cronTaskVerifyObjectName, s.cronTaskVerifyObject, cron.TaskDefinitionOneTimeJob, cronTaskVerifyObjectArgsFactory)
	crn.RegisterTask(cronTaskUploadObjectName, s.cronTaskUploadObject, cron.TaskDefinitionOneTimeJob, cronTaskUploadObjectArgsFactory)

	return nil
}

func (s *SyncServiceDefault) cronTaskVerifyObject(args any) error {
	return cronTaskVerifyObject(args.(*cronTaskVerifyObjectArgs), s)
}

func (s *SyncServiceDefault) cronTaskUploadObject(args any) error {
	return cronTaskUploadObject(args.(*cronTaskUploadObjectArgs), s)
}

func (s *SyncServiceDefault) ScheduleJobs(crn cron.CronService) error {
	return nil
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

	proofReader, err := s.storage.DownloadObjectProof(ctx, syncProto, upload.Hash)

	if err != nil {
		return err
	}

	proof, err := io.ReadAll(proofReader)

	meta := FileMeta{
		Hash:      upload.Hash,
		Proof:     proof,
		Multihash: nil,
		Protocol:  upload.Protocol,
		Key:       object.Key,
		Size:      uint64(object.Size),
		Slabs:     object.Slabs,
	}

	err = s.grpcPlugin.Update(meta)

	if err != nil {
		return err
	}

	return nil
}

func (s *SyncServiceDefault) LogKey() []byte {
	return s.logKey
}

func (s *SyncServiceDefault) Import(object string, uploaderID uint64) error {
	protos := registry.GetAllProtocols()
	ctx := context.Background()
	for _, proto := range protos {
		syncProto, ok := proto.(SyncProtocol)
		if !ok {
			continue
		}

		if syncProto.ValidIdentifier(object) {
			meta, err := s.grpcPlugin.Query([]string{object})

			if err != nil {
				return err
			}

			if len(meta) == 0 {
				return errors.New("object not found")
			}

			upload, err := s.metadata.GetUpload(ctx, meta[0].Hash)
			if err == nil || !upload.IsEmpty() {
				return errors.New("object already exists")
			}

			metaDeref := make([]FileMeta, 0)
			for _, m := range meta {
				metaDeref = append(metaDeref, *m)
			}

			err = s.cron.CreateJobIfNotExists(cronTaskVerifyObjectName, cronTaskVerifyObjectArgs{Object: metaDeref, UploaderID: uploaderID}, []string{object})
			if err != nil {
				return err
			}

			return nil
		}
	}

	return errors.New("invalid object")
}

func (s *SyncServiceDefault) init() error {
	s.cron.RegisterService(s)

	/*temp, err := os.CreateTemp(os.TempDir(), "sync")
	  if err != nil {
	  	return err
	  }

	  err = temp.Chmod(1755)
	  if err != nil {
	  	return err
	  }

	  _, err = io.Copy(temp, bytes.NewReader(pluginBin))
	  if err != nil {
	  	return err
	  }

	  err = temp.Close()
	  if err != nil {
	  	return err
	  }*/

	cwd, err := os.Getwd()

	if err != nil {
		return err

	}

	cmd := exec.Command("/root/.nvm/versions/node/v21.7.1/bin/node", path.Join(cwd, "./sync/node/app/src/index.js"))
	cmd.Env = append(os.Environ(), "NODE_NO_WARNINGS=1")
	cmd.Dir = path.Join(cwd, "./sync/node/app")
	clientInst := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion: 1,
		},
		Plugins: plugin.PluginSet{
			"sync": &syncGrpcPlugin{},
		},
		Cmd:              cmd,
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})

	s.grpcClient = clientInst

	rpcClient, err := clientInst.Client()
	if err != nil {
		return err
	}

	pluginInst, err := rpcClient.Dispense("sync")
	if err != nil {
		return err
	}

	s.grpcPlugin = pluginInst.(sync)

	ret, err := s.grpcPlugin.Init(s.identity)
	if err != nil {
		return err
	}

	s.logKey = ret.GetLogKey()

	return nil
}

func (s *SyncServiceDefault) stop() error {
	s.grpcClient.Kill()

	return nil
}

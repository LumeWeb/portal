package sync

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"embed"
	"errors"
	"io"
	"os"
	"os/exec"
	"path"

	"golang.org/x/crypto/hkdf"

	"github.com/samber/lo"

	"github.com/hanwen/go-fuse/v2/fuse"

	gfs "github.com/hanwen/go-fuse/v2/fs"

	go_fuse_embed "github.com/LumeWeb/go-fuse-embed"

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
//go:generate bash -c "cd node && bash build.sh"
//go:generate go run download_node.go

//go:embed node/app
var nodeServer embed.FS

const nodeEmbedPrefix = "node/app"
const syncDataFolder = "sync_data"

type SyncServiceDefault struct {
	config          *config.Manager
	renter          *renter.RenterDefault
	metadata        metadata.MetadataService
	storage         storage.StorageService
	cron            *cron.CronServiceDefault
	logger          *zap.Logger
	grpcClient      *plugin.Client
	grpcPlugin      sync
	identity        ed25519.PrivateKey
	logKey          []byte
	syncServerMount *fuse.Server
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
	HashFromIdentifier(string) ([]byte, error)
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
	crn.RegisterTask(cronTaskScanObjectsName, s.cronTaskScanObjects, cronTaskScanObjectsDefinition, cronTaskScanObjectsArgsFactory)

	return nil
}

func (s *SyncServiceDefault) cronTaskVerifyObject(args any) error {
	return cronTaskVerifyObject(args.(*cronTaskVerifyObjectArgs), s)
}

func (s *SyncServiceDefault) cronTaskUploadObject(args any) error {
	return cronTaskUploadObject(args.(*cronTaskUploadObjectArgs), s)
}

func (s *SyncServiceDefault) cronTaskScanObjects(args any) error {
	return cronTaskScanObjects(args.(*cronTaskScanObjectsArgs), s)
}

func (s *SyncServiceDefault) ScheduleJobs(crn cron.CronService) error {
	err := crn.CreateJobIfNotExists(cronTaskScanObjectsName, nil, nil)
	if err != nil {
		return err
	}
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

	noShards := false

	for _, slab := range object.Slabs {
		if len(slab.Shards) == 0 {
			noShards = true
			break
		}
	}

	if noShards {
		s.logger.Debug("object has at-least one slab with no shards", zap.String("hash", fileName))
		return nil
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
			hash, err := syncProto.HashFromIdentifier(object)
			if err != nil {
				return err
			}
			meta, err := s.grpcPlugin.Query([]string{object})

			if err != nil {
				return err
			}

			meta = lo.Filter(meta, func(m *FileMeta, _ int) bool {
				noShards := false
				for _, slab := range m.Slabs {
					if len(slab.Shards) == 0 {
						noShards = true
						break
					}
				}

				return !noShards
			})

			if len(meta) == 0 {
				return errors.New("object not found")
			}

			upload, err := s.metadata.GetUpload(ctx, hash)
			if err == nil || !upload.IsEmpty() {
				return errors.New("object already exists")
			}

			metaDeref := make([]FileMeta, 0)
			for _, m := range meta {
				metaDeref = append(metaDeref, *m)
			}

			err = s.cron.CreateJobIfNotExists(cronTaskVerifyObjectName, cronTaskVerifyObjectArgs{
				Hash:       hash,
				Object:     metaDeref,
				UploaderID: uploaderID,
			}, []string{object})
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
	fuseFs := go_fuse_embed.New(&nodeServer, nodeEmbedPrefix)
	fuseFs.ChmodFile("/node", 0555)

	mountDir, err := os.MkdirTemp("", "")
	if err != nil {
		return err
	}

	server, err := gfs.Mount(mountDir, fuseFs, &gfs.Options{})
	if err != nil {
		return err
	}

	err = server.WaitMount()
	if err != nil {
		return err
	}

	nodePath := path.Join(mountDir, "node")
	appPath := path.Join(mountDir, "app/app/bundle.js")

	cmd := exec.Command(nodePath, appPath)
	cmd.Env = append(os.Environ(), "NODE_NO_WARNINGS=1")
	cmd.Dir = mountDir
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

	go func() {
		err := func() error {
			rpcClient, err := clientInst.Client()
			if err != nil {

				return err
			}

			pluginInst, err := rpcClient.Dispense("sync")
			if err != nil {
				return err
			}

			s.grpcPlugin = pluginInst.(sync)

			dataDir := path.Join(path.Dir(s.config.Viper().ConfigFileUsed()), syncDataFolder)

			hasher := hkdf.New(sha256.New, s.identity, s.config.Config().Core.NodeID.Bytes(), []byte("sync"))
			derivedSeed := make([]byte, 32)

			if _, err := io.ReadFull(hasher, derivedSeed); err != nil {
				s.logger.Fatal("failed to generate child key seed", zap.Error(err))
			}

			nodeKey := ed25519.NewKeyFromSeed(derivedSeed)

			ret, err := s.grpcPlugin.Init(s.identity, nodeKey, dataDir)
			if err != nil {
				return err
			}

			s.logKey = ret.GetLogKey()
			return nil
		}()
		if err != nil {
			s.logger.Fatal("failed to start sync service", zap.Error(err))
		}
	}()

	return nil
}

func (s *SyncServiceDefault) stop() error {
	if s.syncServerMount != nil {
		err := s.syncServerMount.Unmount()
		if err != nil {
			return err
		}
	}

	return nil
}

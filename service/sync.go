package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"github.com/hashicorp/go-plugin"
	"github.com/samber/lo"
	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	node_server "go.lumeweb.com/portal-plugin-sync-node-server/go"
	"go.lumeweb.com/portal/config"
	"go.lumeweb.com/portal/config/types"
	"go.lumeweb.com/portal/core"
	"go.lumeweb.com/portal/service/internal/sync"
	"go.uber.org/zap"
	"golang.org/x/crypto/hkdf"
	"io"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"
)

var _ core.CronableService = (*SyncServiceDefault)(nil)

func init() {
	core.RegisterService(core.ServiceInfo{
		ID: core.SYNC_SERVICE,
		Factory: func() (core.Service, []core.ContextBuilderOption, error) {
			return NewSyncService()
		},
		Depends: []string{core.RENTER_SERVICE, core.STORAGE_SERVICE, core.METADATA_SERVICE, core.CRON_SERVICE},
	})
}

const ETC_NODE_PREFIX = "/node/"
const ETC_NODE_PLACEHOLDER = "%s"
const ETC_NODE_SYNC_SUFFIX = "/sync"
const ETC_SYNC_PREFIX = ETC_NODE_PREFIX + ETC_NODE_PLACEHOLDER + ETC_NODE_SYNC_SUFFIX
const ETC_SYNC_BOOTSTRAP_KEY = "/sync/bootstrap"
const ETC_SYNC_LEADER_ELECTION_KEY = "/sync/leader"

const syncDataFolder = "sync_data"

type SyncServiceDefault struct {
	ctx        core.Context
	config     config.Manager
	logger     *core.Logger
	grpcClient *plugin.Client
	grpcPlugin sync.Sync
	logKey     []byte
	renter     core.RenterService
	storage    core.StorageService
	metadata   core.MetadataService
	cron       core.CronService
}

type SyncProtocol interface {
	Name() string
	EncodeFileName([]byte) string
	ValidIdentifier(string) bool
	HashFromIdentifier(string) ([]byte, error)
	StorageProtocol() core.StorageProtocol
}

func NewSyncService() (*SyncServiceDefault, []core.ContextBuilderOption, error) {
	_sync := &SyncServiceDefault{}

	opts := core.ContextOptions(
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			_sync.ctx = ctx
			_sync.config = ctx.Config()
			_sync.logger = ctx.Logger()
			_sync.renter = ctx.Service(core.RENTER_SERVICE).(core.RenterService)
			_sync.storage = ctx.Service(core.STORAGE_SERVICE).(core.StorageService)
			_sync.metadata = ctx.Service(core.METADATA_SERVICE).(core.MetadataService)
			_sync.cron = ctx.Service(core.CRON_SERVICE).(core.CronService)
			return nil
		}),
		core.ContextWithStartupFunc(func(ctx core.Context) error {
			if true {
				//if ctx.Config().Config().Core.Sync.Enabled {
				err := _sync.init()
				if err != nil {
					_sync.logger.Error("failed to initialize sync service", zap.Error(err))
				}

				return err
			}

			return nil
		}),
		core.ContextWithExitFunc(func(ctx core.Context) error {
			return _sync.stop()
		}),
	)

	return _sync, opts, nil
}

func (s *SyncServiceDefault) RegisterTasks(crn core.CronService) error {
	crn.RegisterTask(sync.CronTaskVerifyObjectName, sync.CronTaskVerifyObject, core.CronTaskDefinitionOneTimeJob, core.CronTaskNoArgsFactory)
	crn.RegisterTask(sync.CronTaskUploadObjectName, sync.CronTaskUploadObject, core.CronTaskDefinitionOneTimeJob, sync.CronTaskUploadObjectArgsFactory)
	crn.RegisterTask(sync.CronTaskScanObjectsName, sync.CronTaskScanObjects, sync.CronTaskScanObjectsDefinition, core.CronTaskNoArgsFactory)

	return nil
}
func (s *SyncServiceDefault) ScheduleJobs(crn core.CronService) error {
	err := crn.CreateJobIfNotExists(sync.CronTaskScanObjectsName, nil, nil)
	if err != nil {
		return err
	}
	return nil
}

func (s *SyncServiceDefault) Update(upload core.UploadMetadata) error {
	if !s.Enabled() {
		return nil
	}

	proto := core.GetProtocol(upload.Protocol)

	if proto == nil {
		return errors.New("protocol not found")
	}

	syncProto, ok := proto.(SyncProtocol)
	if !ok {
		return errors.New("protocol is not a sync protocol")
	}

	fileName := syncProto.EncodeFileName(upload.Hash)

	object, err := s.renter.GetObjectMetadata(s.ctx, upload.Protocol, fileName)
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

	proofReader, err := s.storage.DownloadObjectProof(s.ctx, syncProto, upload.Hash)

	if err != nil {
		return err
	}

	proof, err := io.ReadAll(proofReader)

	meta := sync.FileMeta{
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
	protos := core.GetProtocols()
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

			meta = lo.Filter(meta, func(m *sync.FileMeta, _ int) bool {
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

			_upload, err := s.metadata.GetUpload(ctx, hash)
			if err == nil || !_upload.IsEmpty() {
				return errors.New("object already exists")
			}

			metaDeref := make([]sync.FileMeta, 0)
			for _, m := range meta {
				metaDeref = append(metaDeref, *m)
			}

			err = s.cron.CreateJobIfNotExists(sync.CronTaskVerifyObjectName, sync.CronTaskVerifyObjectArgs{
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
	extractDir, err := os.MkdirTemp(os.TempDir(), "")
	if err != nil {
		return err
	}

	err = unzip(node_server.GetBundle(), extractDir, s.logger)

	if err != nil {
		return err
	}

	nodePath := path.Join(extractDir, "app", "node")
	appPath := path.Join(extractDir, "app", "app", "app", "bundle.js")

	err = os.Chmod(nodePath, 0755)
	if err != nil {
		return err
	}

	cmd := exec.Command(nodePath, appPath)
	cmd.Env = append(os.Environ(), "NODE_NO_WARNINGS=1")
	cmd.Dir = extractDir
	clientInst := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig: plugin.HandshakeConfig{
			ProtocolVersion: 1,
		},
		Plugins: plugin.PluginSet{
			"sync": &sync.SyncGrpcPlugin{},
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

	s.grpcPlugin = pluginInst.(sync.Sync)

	dataDir := path.Join(path.Dir(s.config.ConfigFile()), syncDataFolder)

	hasher := hkdf.New(sha256.New, s.ctx.Config().Config().Core.Identity.PrivateKey(), s.config.Config().Core.NodeID.Bytes(), []byte("sync"))
	derivedSeed := make([]byte, 32)

	if _, err := io.ReadFull(hasher, derivedSeed); err != nil {
		s.logger.Fatal("failed to generate child key seed", zap.Error(err))
	}

	nodeKey := ed25519.NewKeyFromSeed(derivedSeed)

	bootstrap := true
	var client *clientv3.Client

	if s.config.Config().Core.ClusterEnabled() {
		client, err = s.config.Config().Core.Clustered.Etcd.Client()
		if err != nil {
			return err
		}

		// Check if the bootstrap key exists
		resp, err := client.Get(context.Background(), ETC_SYNC_BOOTSTRAP_KEY)
		if err != nil {
			return err
		}

		if resp.Count > 0 {
			// Bootstrap key already exists, no need to bootstrap
			bootstrap = false
		} else {
			// Create a new session
			session, err := concurrency.NewSession(client)
			if err != nil {
				return err
			}
			defer func(session *concurrency.Session) {
				err := session.Close()
				if err != nil {
					s.logger.Error("failed to close etcd session", zap.Error(err))
				}
			}(session)

			// Create a new election
			election := concurrency.NewElection(session, ETC_SYNC_LEADER_ELECTION_KEY)

			// Check if a leader is already elected
			_, err = election.Leader(context.Background())
			if err == nil {
				// Leader already exists, no need to participate in the election
				bootstrap = false
			} else if err == concurrency.ErrElectionNoLeader {
				// No leader exists, participate in the leader election with a timeout
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()

				err := election.Campaign(ctx, s.config.Config().Core.NodeID.String())
				if err != nil {
					if err == context.DeadlineExceeded {
						// Timeout occurred, node did not become the leader
						bootstrap = false
					} else {
						// Other error occurred
						return err
					}
				} else {
					// Successfully elected as the leader
					bootstrap = true
					defer func(election *concurrency.Election, ctx context.Context) {
						err := election.Resign(ctx)
						if err != nil {
							s.logger.Error("failed to resign from leader election", zap.Error(err))
						}
					}(election, context.Background())

					// Set the bootstrap key to the node ID
					_, err = client.Put(context.Background(), ETC_SYNC_BOOTSTRAP_KEY, s.config.Config().Core.NodeID.String())
					if err != nil {
						return err
					}
				}
			} else {
				return err
			}
		}
	}

	var logPubKey ed25519.PublicKey

	if !bootstrap {
		var bootstrapNodeId string
		var boostrapNodeKey ed25519.PrivateKey

		if s.config.Config().Core.ClusterEnabled() {
			resp, err := client.Get(context.Background(), ETC_SYNC_BOOTSTRAP_KEY)
			if err != nil {
				return err
			}

			bootstrapNodeId = string(resp.Kvs[0].Value)

			uuid, err := types.ParseUUID(bootstrapNodeId)
			if err != nil {
				return err
			}

			boostrapHasher := hkdf.New(sha256.New, s.ctx.Config().Config().Core.Identity.PrivateKey(), uuid.Bytes(), []byte("sync"))
			derivedBootstrapSeed := make([]byte, 32)

			if _, err := io.ReadFull(boostrapHasher, derivedBootstrapSeed); err != nil {
				s.logger.Fatal("failed to generate child key seed", zap.Error(err))
			}

			boostrapNodeKey = ed25519.NewKeyFromSeed(derivedBootstrapSeed)
		} else {
			boostrapNodeKey = nodeKey
		}

		logPubKey, err = sync.NodeKey(boostrapNodeKey.Public().([]byte), nil)
		if err != nil {
			return err
		}
	}

	err = s.grpcPlugin.Init(logPubKey, nodeKey, dataDir)
	if err != nil {
		return err
	}

	s.logKey = sync.AutoBaseKey(nodeKey.Public().(ed25519.PublicKey))

	if s.config.Config().Core.ClusterEnabled() {
		lease := clientv3.NewLease(client)
		ttl := int64((time.Hour * 24).Seconds())
		grantResp, err := lease.Grant(context.Background(), ttl) // 60 seconds TTL
		if err != nil {
			return err
		}

		pubKey := nodeKey.Public().(ed25519.PublicKey)
		_, err = client.Put(context.Background(), fmt.Sprintf(ETC_SYNC_PREFIX, s.config.Config().Core.NodeID.String()), string(pubKey), clientv3.WithLease(grantResp.ID))
		if err != nil {
			return err
		}

		nodes, err := fetchSyncNodes(client)
		if err != nil {
			return err
		}

		err = s.grpcPlugin.UpdateNodes(nodes)

		if err != nil {
			return err
		}

		go watchExpiringNodes(client, s.logger, func(nodeID types.UUID) {
			if nodeID == s.config.Config().Core.NodeID {
				err := s.grpcPlugin.RemoveNode(pubKey)
				if err != nil {
					s.logger.Error("failed to remove node", zap.Error(err))
				}
			}
		})
	}

	return nil
}

func (s *SyncServiceDefault) stop() error {
	return nil
}

func (s *SyncServiceDefault) Enabled() bool {
	return true
	//	return s.config.Config().Core.Sync.Enabled
}

func unzip(data []byte, dest string, logger *core.Logger) error {
	read, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, file := range read.File {
		if file.Mode().IsDir() {
			continue
		}
		open, err := file.Open()
		if err != nil {
			return err
		}
		name := path.Join(dest, file.Name)
		err = os.MkdirAll(path.Dir(name), os.FileMode(0755))
		if err != nil {
			return err
		}
		create, err := os.Create(name)
		if err != nil {
			return err
		}
		defer func(create *os.File) {
			err := create.Close()
			if err != nil {
				logger.Error("failed to close file", zap.Error(err))
			}
		}(create)
		_, err = create.ReadFrom(open)
		if err != nil {
			return err
		}
	}
	return nil
}

func fetchSyncNodes(client *clientv3.Client) ([]ed25519.PublicKey, error) {
	resp, err := client.Get(context.Background(), ETC_NODE_PREFIX, clientv3.WithPrefix())
	if err != nil {
		return nil, err
	}

	var syncNodes []ed25519.PublicKey
	for _, kv := range resp.Kvs {
		syncNodes = append(syncNodes, kv.Value)
	}

	return syncNodes, nil
}
func watchExpiringNodes(client *clientv3.Client, logger *core.Logger, onExpire func(nodeID types.UUID)) {
	watchChan := client.Watch(context.Background(), "/node/", clientv3.WithPrefix())

	for watchResp := range watchChan {
		for _, event := range watchResp.Events {
			if event.Type == clientv3.EventTypeDelete {
				nodeID := strings.TrimPrefix(string(event.Kv.Key), ETC_NODE_PREFIX)
				nodeID = strings.TrimSuffix(nodeID, ETC_NODE_SYNC_SUFFIX)
				nodeUUID, err := types.ParseUUID(nodeID)
				if err != nil {
					logger.Error("failed to parse node ID", zap.Error(err))
					continue
				}
				onExpire(nodeUUID)
			}
		}
	}
}

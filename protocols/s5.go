package protocols

import (
	"context"
	"crypto/ed25519"
	"fmt"
	s5config "git.lumeweb.com/LumeWeb/libs5-go/config"
	s5ed "git.lumeweb.com/LumeWeb/libs5-go/ed25519"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	s5interfaces "git.lumeweb.com/LumeWeb/libs5-go/interfaces"
	s5node "git.lumeweb.com/LumeWeb/libs5-go/node"
	s5storage "git.lumeweb.com/LumeWeb/libs5-go/storage"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"git.lumeweb.com/LumeWeb/portal/storage"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"time"
)

var (
	_ s5interfaces.ProviderStore = (*S5ProviderStore)(nil)
	_ registry.Protocol          = (*S5Protocol)(nil)
)

type S5Protocol struct {
	node          s5interfaces.Node
	config        *viper.Viper
	logger        *zap.Logger
	storage       *storage.StorageServiceImpl
	identity      ed25519.PrivateKey
	providerStore *S5ProviderStore
}

type S5ProtocolParams struct {
	fx.In
	Config        *viper.Viper
	Logger        *zap.Logger
	Storage       *storage.StorageServiceImpl
	Identity      ed25519.PrivateKey
	ProviderStore *S5ProviderStore
}

type S5ProtocolResult struct {
	fx.Out
	Protocol registry.Protocol `group:"protocol"`
}

var S5ProtocolModule = fx.Module("s5_protocol",
	fx.Provide(NewS5Protocol),
	fx.Provide(func(protocol *S5Protocol) *S5ProviderStore {
		return &S5ProviderStore{proto: protocol}
	}),
)

func NewS5Protocol(
	params S5ProtocolParams,
) (S5ProtocolResult, error) {
	return S5ProtocolResult{
		Protocol: &S5Protocol{
			config:        params.Config,
			logger:        params.Logger,
			storage:       params.Storage,
			identity:      params.Identity,
			providerStore: params.ProviderStore,
		},
	}, nil
}

func InitS5Protocol(s5 *S5Protocol) error {
	return s5.Init()
}
func (s *S5Protocol) Init() error {
	cfg := &s5config.NodeConfig{
		P2P: s5config.P2PConfig{
			Network: "",
			Peers:   s5config.PeersConfig{Initial: []string{}},
		},
		KeyPair: s5ed.New(s.identity),
		DB:      nil,
		Logger:  s.logger.Named("s5"),
		HTTP:    s5config.HTTPConfig{},
	}

	pconfig := s.config.Sub("protocol.s5")

	if pconfig == nil {
		s.logger.Fatal("Missing protocol.s5 Config")
	}

	err := pconfig.Unmarshal(cfg)
	if err != nil {
		return err
	}

	cfg.HTTP.API.Domain = fmt.Sprintf("s5.%s", s.config.GetString("core.domain"))

	if s.config.IsSet("core.externalPort") {
		cfg.HTTP.API.Port = s.config.GetUint("core.externalPort")
	} else {
		cfg.HTTP.API.Port = s.config.GetUint("core.port")
	}

	dbPath := pconfig.GetString("dbPath")

	if dbPath == "" {
		s.logger.Fatal("protocol.s5.dbPath is required")
	}

	_, p, err := ed25519.GenerateKey(nil)
	if err != nil {
		s.logger.Fatal("Failed to generate key", zap.Error(err))
	}

	cfg.KeyPair = s5ed.New(p)

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		s.logger.Fatal("Failed to open db", zap.Error(err))
	}

	cfg.DB = db

	s.node = s5node.NewNode(cfg)

	s.node.SetProviderStore(s.providerStore)

	return nil
}
func (s *S5Protocol) Start(ctx context.Context) error {
	err := s.node.Start()
	if err != nil {
		return err
	}

	identity, err := s.node.Services().P2P().NodeId().ToString()

	if err != nil {
		return err
	}

	s.logger.Info("S5 protocol started", zap.String("identity", identity), zap.String("network", s.node.NetworkId()), zap.String("domain", s.node.Config().HTTP.API.Domain))

	return nil
}

func (s *S5Protocol) Name() string {
	return "s5"
}

func (s *S5Protocol) Stop(ctx context.Context) error {
	return nil
}

func (s *S5Protocol) Node() s5interfaces.Node {
	return s.node
}

type S5ProviderStore struct {
	proto *S5Protocol
}

func (s S5ProviderStore) CanProvide(hash *encoding.Multihash, kind []types.StorageLocationType) bool {
	for _, t := range kind {
		switch t {
		case types.StorageLocationTypeArchive, types.StorageLocationTypeFile, types.StorageLocationTypeFull:
			rawHash := hash.HashBytes()

			if exists, upload := s.proto.storage.TusUploadExists(rawHash); exists {
				if upload.Completed {
					return true
				}

			}
			if exists, _ := s.proto.storage.FileExists(rawHash); exists {
				return true
			}
		}
	}
	return false
}

func (s S5ProviderStore) Provide(hash *encoding.Multihash, kind []types.StorageLocationType) (s5interfaces.StorageLocation, error) {
	for _, t := range kind {
		if !s.CanProvide(hash, []types.StorageLocationType{t}) {
			continue
		}

		switch t {
		case types.StorageLocationTypeArchive:
			return s5storage.NewStorageLocation(int(types.StorageLocationTypeArchive), []string{}, calculateExpiry(24*time.Hour)), nil
		case types.StorageLocationTypeFile, types.StorageLocationTypeFull:
			return s5storage.NewStorageLocation(int(types.StorageLocationTypeFull), []string{generateDownloadUrl(hash, s.proto.config, s.proto.logger)}, calculateExpiry(24*time.Hour)), nil
		}
	}

	hashStr, err := hash.ToString()
	if err != nil {
		return nil, err
	}

	return nil, fmt.Errorf("could not provide hash %s for types %v", hashStr, kind)
}
func calculateExpiry(duration time.Duration) int64 {
	return time.Now().Add(duration).Unix()
}

func generateDownloadUrl(hash *encoding.Multihash, config *viper.Viper, logger *zap.Logger) string {
	domain := config.GetString("core.domain")

	hashStr, err := hash.ToBase64Url()
	if err != nil {
		logger.Error("error encoding hash", zap.Error(err))
	}

	return fmt.Sprintf("https://s5.%s/s5/download/%s", domain, hashStr)
}

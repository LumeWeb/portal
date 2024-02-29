package s5

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"
	"io"
	"time"

	"git.lumeweb.com/LumeWeb/portal/config"
	"golang.org/x/crypto/hkdf"

	"git.lumeweb.com/LumeWeb/portal/metadata"

	"git.lumeweb.com/LumeWeb/portal/storage"

	s5config "git.lumeweb.com/LumeWeb/libs5-go/config"
	s5ed "git.lumeweb.com/LumeWeb/libs5-go/ed25519"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	s5fx "git.lumeweb.com/LumeWeb/libs5-go/fx"
	s5node "git.lumeweb.com/LumeWeb/libs5-go/node"
	s5storage "git.lumeweb.com/LumeWeb/libs5-go/storage"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/protocols/registry"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	_ s5storage.ProviderStore = (*S5ProviderStore)(nil)
	_ registry.Protocol       = (*S5Protocol)(nil)
	_ storage.StorageProtocol = (*S5Protocol)(nil)
)

type S5Protocol struct {
	portalConfig *config.Manager
	config       *Config
	logger       *zap.Logger
	storage      storage.StorageService
	identity     ed25519.PrivateKey
	node         *s5node.Node
	tusHandler   *TusHandler
	store        *S5ProviderStore
}

type S5ProtocolParams struct {
	fx.In
	PortalConfig  *config.Manager
	Logger        *zap.Logger
	Storage       storage.StorageService
	Identity      ed25519.PrivateKey
	ProviderStore *S5ProviderStore
	TusHandler    *TusHandler
}

type S5ProtocolResult struct {
	fx.Out
	Protocol     registry.Protocol `group:"protocol"`
	S5Protocol   *S5Protocol
	S5NodeConfig *s5config.NodeConfig
	Db           *bolt.DB
}

type S5ProviderStoreParams struct {
	fx.In
	Config   *config.Manager
	Metadata metadata.MetadataService
	Logger   *zap.Logger
	Tus      *TusHandler
}

var ProtocolModule = fx.Module("s5_api",
	fx.Provide(NewS5Protocol),
	fx.Provide(NewTusHandler),
	fx.Provide(NewS5ProviderStore),
	fx.Replace(func(cfg *s5config.NodeConfig) *zap.Logger {
		return cfg.Logger
	}),
	s5fx.Module,
)

var PreInit = func(protocol *S5Protocol, node *s5node.Node) {
	protocol.SetNode(node)
}

func NewS5Protocol(
	params S5ProtocolParams,
) (S5ProtocolResult, error) {
	proto := &S5Protocol{
		portalConfig: params.PortalConfig,
		logger:       params.Logger,
		storage:      params.Storage,
		identity:     params.Identity,
		tusHandler:   params.TusHandler,
		store:        params.ProviderStore,
	}

	cfg, err := configureS5Protocol(proto)
	if err != nil {
		return S5ProtocolResult{}, err
	}

	return S5ProtocolResult{
		Protocol:     proto,
		S5Protocol:   proto,
		S5NodeConfig: cfg,
		Db:           cfg.DB,
	}, nil
}

func configureS5Protocol(proto *S5Protocol) (*s5config.NodeConfig, error) {
	cfg := proto.Config().(*Config)
	cm := proto.portalConfig
	portalCfg := cm.Config()
	vpr := cm.Viper()

	err := cm.ConfigureProtocol(proto.Name(), cfg)
	if err != nil {
		return nil, err
	}

	cfg.HTTP.API.Domain = fmt.Sprintf("s5.%s", vpr.GetString("core.domain"))

	if portalCfg.Core.ExternalPort != 0 {
		cfg.HTTP.API.Port = portalCfg.Core.ExternalPort
	} else {
		cfg.HTTP.API.Port = portalCfg.Core.Port
	}

	if cfg.DbPath == "" {
		proto.logger.Fatal("protocol.s5.db_path is required")
	}

	hasher := hkdf.New(sha256.New, proto.identity, nil, []byte("s5"))
	derivedSeed := make([]byte, 32)

	if _, err := io.ReadFull(hasher, derivedSeed); err != nil {
		proto.logger.Fatal("Failed to generate child key seed", zap.Error(err))
		return nil, err
	}

	p := ed25519.NewKeyFromSeed(derivedSeed)
	cfg.KeyPair = s5ed.New(p)

	db, err := bolt.Open(cfg.DbPath, 0600, nil)
	if err != nil {
		proto.logger.Fatal("Failed to open db", zap.Error(err))
	}

	cfg.DB = db

	cfg.Logger = proto.logger.Named("s5")

	return cfg.NodeConfig, nil
}

func (s *S5Protocol) Config() config.ProtocolConfig {
	if s.config == nil {
		s.config = &Config{
			NodeConfig: &s5config.NodeConfig{},
		}
	}

	return s.config
}
func NewS5ProviderStore(params S5ProviderStoreParams) *S5ProviderStore {
	return &S5ProviderStore{
		config:   params.Config,
		logger:   params.Logger,
		tus:      params.Tus,
		metadata: params.Metadata,
	}
}

func (s *S5Protocol) Init(ctx context.Context) error {
	s.node.Services().Storage().SetProviderStore(s.store)

	err := s.node.Init(ctx)
	if err != nil {
		return err
	}

	s.tusHandler.SetStorageProtocol(GetStorageProtocol(s))

	err = s.tusHandler.Init()
	if err != nil {
		return err
	}

	return nil
}
func (s *S5Protocol) Start(ctx context.Context) error {
	err := s.node.Start(ctx)
	if err != nil {
		return err
	}

	identity, err := s.node.NodeId().ToString()

	if err != nil {
		return err
	}

	s.logger.Info("S5 protocol started", zap.String("identity", identity), zap.String("network", s.node.NetworkId()), zap.String("domain", s.node.Config().HTTP.API.Domain))

	return nil
}

func (s *S5Protocol) Name() string {
	return "s5"
}

func (s *S5Protocol) Node() *s5node.Node {
	return s.node
}

func (s *S5Protocol) Stop(ctx context.Context) error {
	return nil
}

func (s *S5Protocol) SetNode(node *s5node.Node) {
	s.node = node
}

func (s *S5Protocol) EncodeFileName(bytes []byte) string {
	bytes = append([]byte{byte(types.HashTypeBlake3)}, bytes...)

	hash, err := encoding.NewMultihash(bytes).ToBase64Url()
	if err != nil {
		s.logger.Error("error encoding hash", zap.Error(err))
		panic(err)
	}

	return hash
}

type S5ProviderStore struct {
	config   *config.Manager
	logger   *zap.Logger
	tus      *TusHandler
	metadata metadata.MetadataService
}

func (s S5ProviderStore) CanProvide(hash *encoding.Multihash, kind []types.StorageLocationType) bool {
	ctx := context.Background()
	for _, t := range kind {
		switch t {
		case types.StorageLocationTypeArchive, types.StorageLocationTypeFile, types.StorageLocationTypeFull:
			rawHash := hash.HashBytes()

			if exists, upload := s.tus.UploadExists(ctx, rawHash); exists {
				if upload.Completed {
					return true
				}

			}
			if _, err := s.metadata.GetUpload(ctx, rawHash); err == nil {
				return true
			}
		}
	}
	return false
}

func (s S5ProviderStore) Provide(hash *encoding.Multihash, kind []types.StorageLocationType) (s5storage.StorageLocation, error) {
	for _, t := range kind {
		if !s.CanProvide(hash, []types.StorageLocationType{t}) {
			continue
		}

		switch t {
		case types.StorageLocationTypeArchive:
			return s5storage.NewStorageLocation(int(types.StorageLocationTypeArchive), []string{}, calculateExpiry(24*time.Hour)), nil
		case types.StorageLocationTypeFile, types.StorageLocationTypeFull:
			return s5storage.NewStorageLocation(int(types.StorageLocationTypeFull), []string{generateDownloadUrl(hash, s.config, s.logger), generateProofUrl(hash, s.config, s.logger)}, calculateExpiry(1*time.Hour)), nil
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

func generateDownloadUrl(hash *encoding.Multihash, config *config.Manager, logger *zap.Logger) string {
	domain := config.Config().Core.Domain

	hashStr, err := hash.ToBase64Url()
	if err != nil {
		logger.Error("error encoding hash", zap.Error(err))
	}

	return fmt.Sprintf("https://s5.%s/s5/download/%s", domain, hashStr)
}

func generateProofUrl(hash *encoding.Multihash, config *config.Manager, logger *zap.Logger) string {
	domain := config.Config().Core.Domain

	hashStr, err := hash.ToBase64Url()
	if err != nil {
		logger.Error("error encoding hash", zap.Error(err))
	}

	return fmt.Sprintf("https://s5.%s/s5/download/%s.obao", domain, hashStr)
}

package protocols

import (
	"crypto/ed25519"
	"fmt"
	s5config "git.lumeweb.com/LumeWeb/libs5-go/config"
	s5ed "git.lumeweb.com/LumeWeb/libs5-go/ed25519"
	"git.lumeweb.com/LumeWeb/libs5-go/encoding"
	s5interfaces "git.lumeweb.com/LumeWeb/libs5-go/interfaces"
	s5node "git.lumeweb.com/LumeWeb/libs5-go/node"
	s5storage "git.lumeweb.com/LumeWeb/libs5-go/storage"
	"git.lumeweb.com/LumeWeb/libs5-go/types"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
	"time"
)

var (
	_ interfaces.Protocol        = (*S5Protocol)(nil)
	_ s5interfaces.ProviderStore = (*S5ProviderStore)(nil)
)

type S5Protocol struct {
	node   s5interfaces.Node
	portal interfaces.Portal
}

func NewS5Protocol() *S5Protocol {
	return &S5Protocol{}
}

func (s *S5Protocol) Initialize(portal interfaces.Portal) error {
	s.portal = portal

	logger := portal.Logger()
	config := portal.Config()

	cfg := &s5config.NodeConfig{
		P2P: s5config.P2PConfig{
			Network: "",
			Peers:   s5config.PeersConfig{Initial: []string{}},
		},
		KeyPair: s5ed.New(portal.Identity()),
		DB:      nil,
		Logger:  portal.Logger().Named("s5"),
		HTTP:    s5config.HTTPConfig{},
	}

	pconfig := config.Sub("protocol.s5")

	if pconfig == nil {
		logger.Fatal("Missing protocol.s5 config")
	}

	err := pconfig.Unmarshal(cfg)
	if err != nil {
		return err
	}

	cfg.HTTP.API.Domain = fmt.Sprintf("s5.%s", config.GetString("core.domain"))

	if config.IsSet("core.externalPort") {
		cfg.HTTP.API.Port = config.GetUint("core.externalPort")
	} else {
		cfg.HTTP.API.Port = config.GetUint("core.port")
	}

	dbPath := pconfig.GetString("dbPath")

	if dbPath == "" {
		logger.Fatal("protocol.s5.dbPath is required")
	}

	_, p, err := ed25519.GenerateKey(nil)
	if err != nil {
		logger.Fatal("Failed to generate key", zap.Error(err))
	}

	cfg.KeyPair = s5ed.New(p)

	db, err := bolt.Open(dbPath, 0600, nil)
	if err != nil {
		logger.Fatal("Failed to open db", zap.Error(err))
	}

	cfg.DB = db

	s.node = s5node.NewNode(cfg)

	s.node.SetProviderStore(&S5ProviderStore{proto: s})

	return nil
}
func (s *S5Protocol) Start() error {
	err := s.node.Start()
	if err != nil {
		return err
	}

	identity, err := s.node.Services().P2P().NodeId().ToString()

	if err != nil {
		return err
	}

	s.portal.Logger().Info("S5 protocol started", zap.String("identity", identity), zap.String("network", s.node.NetworkId()), zap.String("domain", s.node.Config().HTTP.API.Domain))

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
			if exists, _ := s.proto.portal.Storage().FileExists(rawHash); exists {
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
			return s5storage.NewStorageLocation(int(types.StorageLocationTypeArchive), []string{generateDownloadUrl(hash, s.proto.portal)}, calculateExpiry(24*time.Hour)), nil
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

func generateDownloadUrl(hash *encoding.Multihash, portal interfaces.Portal) string {
	domain := portal.Config().GetString("core.domain")

	hashStr, err := hash.ToBase64Url()
	if err != nil {
		portal.Logger().Error("error encoding hash", zap.Error(err))
	}

	return fmt.Sprintf("https://%s/api/s5/download/%s", domain, hashStr)
}

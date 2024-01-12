package protocols

import (
	"crypto/ed25519"
	s5config "git.lumeweb.com/LumeWeb/libs5-go/config"
	s5ed "git.lumeweb.com/LumeWeb/libs5-go/ed25519"
	s5interfaces "git.lumeweb.com/LumeWeb/libs5-go/interfaces"
	s5node "git.lumeweb.com/LumeWeb/libs5-go/node"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/spf13/viper"
	bolt "go.etcd.io/bbolt"
	"go.uber.org/zap"
)

var (
	_ interfaces.Protocol = (*S5Protocol)(nil)
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
		Logger:  logger,
		HTTP:    s5config.HTTPConfig{},
	}

	pconfig := config.Sub("protocol.s5")

	if pconfig != nil {
		err := pconfig.Unmarshal(cfg)
		if err != nil {
			return err
		}
	}

	cfg.HTTP.API.Domain = config.GetString("core.domain")
	cfg.HTTP.API.Port = config.GetUint("core.port")
	dbPath := pconfig.GetString("dbPath")

	if dbPath == "" {
		logger.Fatal("dbPath is required")
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

	return nil
}
func (s *S5Protocol) Start() error {
	return s.node.Start()
}
func (s *S5Protocol) Node() s5interfaces.Node {
	return s.node
}

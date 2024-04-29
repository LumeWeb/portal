package sync

import (
	"context"
	"crypto/ed25519"
	"errors"
	"hash"
	"os"
	"os/exec"
	"path"

	"golang.org/x/crypto/blake2b"

	"github.com/hashicorp/go-plugin"

	"github.com/LumeWeb/portal/config"
	"github.com/LumeWeb/portal/metadata"
	"github.com/LumeWeb/portal/protocols/registry"
	"github.com/LumeWeb/portal/renter"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

//go:generate bash -c "cd proto && buf generate"

type SyncServiceDefault struct {
	config     *config.Manager
	renter     *renter.RenterDefault
	logger     *zap.Logger
	grpcClient *plugin.Client
	grpcPlugin sync
	identity   ed25519.PrivateKey
	logKey     hash.Hash
}

type SyncServiceParams struct {
	fx.In
	Config   *config.Manager
	Renter   *renter.RenterDefault
	Logger   *zap.Logger
	Identity ed25519.PrivateKey
}

type SyncProtocol interface {
	Name() string
	EncodeFileName([]byte) string
}

var Module = fx.Module("sync",
	fx.Options(
		fx.Provide(NewSyncServiceDefault),
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

func NewSyncServiceDefault(params SyncServiceParams) *SyncServiceDefault {
	return &SyncServiceDefault{
		config:   params.Config,
		renter:   params.Renter,
		logger:   params.Logger,
		identity: params.Identity,
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

	meta := FileMeta{
		Hash:      upload.Hash,
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
func (s *SyncServiceDefault) init() error {
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

	cmd := exec.Command("/root/.nvm/versions/node/v21.7.1/bin/node", "--inspect-brk=0.0.0.0", path.Join(cwd, "./sync/node/app/src/index.js"))
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

	s.logKey, err = blake2b.New256(ret.GetDiscoveryKey())

	if err != nil {
		return err
	}

	return nil
}

func (s *SyncServiceDefault) stop() error {
	s.grpcClient.Kill()

	return nil
}

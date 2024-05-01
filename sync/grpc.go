package sync

import (
	"context"
	"crypto/ed25519"

	"github.com/LumeWeb/portal/sync/proto/gen/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

var _ sync = (*syncGRPC)(nil)

type sync interface {
	Init(privateKey ed25519.PrivateKey) (*proto.InitResponse, error)
	Update(meta FileMeta) error
	Query(keys []string) ([]*FileMeta, error)
}

type syncGrpcPlugin struct {
	plugin.Plugin
}

func (p *syncGrpcPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (p *syncGrpcPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &syncGRPC{client: proto.NewSyncClient(c)}, nil
}

type Result struct {
	Hash   []byte
	Proof  []byte
	Length uint
}
type syncGRPC struct {
	client proto.SyncClient
}

func (b *syncGRPC) Init(privateKey ed25519.PrivateKey) (*proto.InitResponse, error) {
	ret, err := b.client.Init(context.Background(), &proto.InitRequest{PrivateKey: privateKey})

	if err != nil {
		return nil, err
	}

	return ret, nil
}
func (b *syncGRPC) Update(meta FileMeta) error {
	_, err := b.client.Update(context.Background(), &proto.UpdateRequest{Data: meta.ToProtobuf()})

	if err != nil {
		return err
	}

	return nil
}

func (b *syncGRPC) Query(keys []string) ([]*FileMeta, error) {
	ret, err := b.client.Query(context.Background(), &proto.QueryRequest{Keys: keys})

	if err != nil {
		return nil, err
	}

	if ret == nil || len(ret.Data) == 0 {
		return nil, nil
	}

	meta := make([]*FileMeta, 0)

	for _, data := range ret.Data {
		fileMeta, err := FileMetaFromProtobuf(data)
		if err != nil {
			return nil, err
		}
		meta = append(meta, fileMeta)
	}

	return meta, nil
}

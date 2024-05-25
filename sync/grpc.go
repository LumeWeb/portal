package sync

import (
	"context"
	"crypto/ed25519"
	"github.com/samber/lo"

	"github.com/LumeWeb/portal/sync/proto/gen/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

var _ sync = (*syncGRPC)(nil)

type sync interface {
	Init(logPrivateKey ed25519.PrivateKey, nodePrivateKey ed25519.PrivateKey, dataDir string) (*proto.InitResponse, error)
	Update(meta FileMeta) error
	Query(keys []string) ([]*FileMeta, error)
	UpdateNodes(nodes []ed25519.PublicKey) error
	RemoveNode(node ed25519.PublicKey) error
}

type syncGrpcPlugin struct {
	plugin.Plugin
}

func (p *syncGrpcPlugin) GRPCServer(_ *plugin.GRPCBroker, _ *grpc.Server) error {
	return nil
}

func (p *syncGrpcPlugin) GRPCClient(_ context.Context, _ *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
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

func (b *syncGRPC) Init(logPrivateKey ed25519.PrivateKey, nodePrivateKey ed25519.PrivateKey, dataDir string) (*proto.InitResponse, error) {
	ret, err := b.client.Init(context.Background(), &proto.InitRequest{LogPrivateKey: logPrivateKey, NodePrivateKey: nodePrivateKey, DataDir: dataDir})

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

func (b *syncGRPC) UpdateNodes(nodes []ed25519.PublicKey) error {
	nodeList := lo.Map[ed25519.PublicKey, []byte](nodes, func(node ed25519.PublicKey, _ int) []byte {
		return node
	})

	ret, err := b.client.UpdateNodes(context.Background(), &proto.UpdateNodesRequest{Nodes: nodeList})

	if err != nil {
		return err
	}

	if ret == nil {
		return nil
	}

	return nil
}

func (b *syncGRPC) RemoveNode(node ed25519.PublicKey) error {
	_, err := b.client.RemoveNode(context.Background(), &proto.RemoveNodeRequest{Node: node})

	if err != nil {
		return err
	}

	return nil
}

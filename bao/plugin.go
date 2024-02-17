package bao

import (
	"context"

	"git.lumeweb.com/LumeWeb/portal/bao/proto"
	"github.com/google/uuid"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

var _ Bao = (*BaoGRPC)(nil)

type Bao interface {
	NewHasher() uuid.UUID
	Hash(id uuid.UUID, data []byte) bool
	Finish(id uuid.UUID) Result
}

type BaoPlugin struct {
	plugin.Plugin
}

func (p *BaoPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (p *BaoPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &BaoGRPC{client: proto.NewBaoClient(c)}, nil
}

type Result struct {
	Hash   []byte
	Proof  []byte
	Length uint
}
type BaoGRPC struct {
	client proto.BaoClient
}

func (b *BaoGRPC) NewHasher() uuid.UUID {
	ret, err := b.client.NewHasher(context.Background(), &proto.NewHasherRequest{})

	if err != nil {
		panic(err)
	}

	id, err := uuid.Parse(ret.Id)

	if err != nil {
		panic(err)
	}

	return id
}

func (b *BaoGRPC) Hash(id uuid.UUID, data []byte) bool {
	ret, err := b.client.Hash(context.Background(), &proto.HashRequest{Id: id.String(), Data: data})

	if err != nil {
		panic(err)
	}

	return ret.Status
}

func (b *BaoGRPC) Finish(id uuid.UUID) Result {
	ret, err := b.client.Finish(context.Background(), &proto.FinishRequest{Id: id.String()})

	if err != nil {
		panic(err)
	}

	return Result{Hash: ret.Hash, Proof: ret.Proof}
}

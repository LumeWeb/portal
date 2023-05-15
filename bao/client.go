package bao

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/bao/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
)

// GRPCClient is an implementation of KV that talks over RPC.
type GRPCClient struct{ client proto.BaoClient }

func (g *GRPCClient) Init() (uint32, error) {
	init, err := g.client.Init(context.Background(), &empty.Empty{})
	if err != nil {
		return 0, err
	}

	return init.Value, nil
}

func (g *GRPCClient) Write(id uint32, data []byte) error {
	_, err := g.client.Write(context.Background(), &proto.WriteRequest{Id: id, Data: data})
	if err != nil {
		return err
	}

	return nil
}

func (g *GRPCClient) Finalize(id uint32) ([]byte, error) {
	tree, err := g.client.Finalize(context.Background(), &wrappers.UInt32Value{Value: id})
	if err != nil {
		return nil, err
	}

	return tree.Value, nil
}

func (g *GRPCClient) Destroy(id uint32) error {
	_, err := g.client.Destroy(context.Background(), &wrappers.UInt32Value{Value: id})
	if err != nil {
		return err
	}

	return nil
}
func (g *GRPCClient) ComputeFile(path string) ([]byte, error) {
	tree, err := g.client.ComputeFile(context.Background(), &wrappers.StringValue{Value: path})
	if err != nil {
		return nil, err
	}

	return tree.Value, nil
}

package bao

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/bao/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type BAOPlugin struct {
	plugin.Plugin
	Impl Bao
}

func (p *BAOPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (b *BAOPlugin) GRPCClient(_ context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &GRPCClient{client: proto.NewBaoClient(c)}, nil
}

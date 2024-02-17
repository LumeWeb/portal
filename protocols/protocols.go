package protocols

import (
	"context"

	"git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"git.lumeweb.com/LumeWeb/portal/protocols/s5"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func RegisterProtocols() {
	registry.Register(registry.ProtocolEntry{
		Key:    "s5",
		Module: s5.ProtocolModule,
	})
}

func BuildProtocols(config *viper.Viper) fx.Option {
	var options []fx.Option
	enabledProtocols := config.GetStringSlice("core.protocols")
	for _, entry := range registry.GetRegistry() {
		if lo.Contains(enabledProtocols, entry.Key) {
			options = append(options, entry.Module)
		}
	}

	options = append(options, fx.Invoke(func(protocols []registry.Protocol) error {
		for _, protocol := range protocols {
			err := protocol.Init()
			if err != nil {
				return err
			}
		}

		return nil
	}))

	return fx.Module("protocols", fx.Options(options...))
}

type LifecyclesParams struct {
	fx.In

	Protocols []registry.Protocol `group:"protocol"`
}

func SetupLifecycles(lifecycle fx.Lifecycle, params LifecyclesParams) error {
	for _, entry := range registry.GetRegistry() {
		for _, protocol := range params.Protocols {
			if protocol.Name() == entry.Key {
				lifecycle.Append(fx.Hook{
					OnStart: func(ctx context.Context) error {
						return protocol.Start(ctx)
					},
					OnStop: func(ctx context.Context) error {
						return protocol.Stop(ctx)
					},
				})
			}
		}
	}

	return nil
}

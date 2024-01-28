package protocols

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func RegisterProtocols() {
	registry.Register(registry.ProtocolEntry{
		Key:      "s5",
		Module:   S5ProtocolModule,
		InitFunc: InitS5Protocol,
	})
}

func BuildProtocols(config *viper.Viper) fx.Option {
	var options []fx.Option
	enabledProtocols := config.GetStringSlice("core.protocols")
	for _, entry := range registry.GetRegistry() {
		if lo.Contains(enabledProtocols, entry.Key) {
			options = append(options, entry.Module)
			if entry.InitFunc != nil {
				options = append(options, fx.Invoke(entry.InitFunc))
			}
		}
	}

	return fx.Module("protocols", fx.Options(options...))
}

type LifecyclesParams struct {
	fx.In

	Protocols []registry.Protocol `group:"protocol"`
}

func SetupLifecycles(lifecycle fx.Lifecycle, params LifecyclesParams) {
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
}

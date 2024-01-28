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

	return fx.Module("protocol", fx.Options(options...))
}

func SetupLifecycles(lifecycle fx.Lifecycle, protocols []registry.Protocol) {
	for _, entry := range registry.GetRegistry() {
		for _, protocol := range protocols {
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

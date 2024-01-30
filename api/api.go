package api

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"git.lumeweb.com/LumeWeb/portal/api/s5"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func RegisterApis() {
	registry.Register(registry.APIEntry{
		Key:      "s5",
		Module:   s5.Module,
		InitFunc: s5.InitAPI,
	})
}

func BuildApis(config *viper.Viper) fx.Option {
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

	return fx.Module("api", fx.Options(options...))
}

type LifecyclesParams struct {
	fx.In

	Protocols []registry.API `group:"protocol"`
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

package api

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"github.com/samber/lo"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func RegisterApis() {
	registry.Register(registry.APIEntry{
		Key:      "s5",
		Module:   S5Module,
		InitFunc: InitS5Api,
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

	return fx.Options(options...)
}

func SetupLifecycles(lifecycle fx.Lifecycle, protocols []registry.API) {
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

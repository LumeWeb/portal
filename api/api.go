package api

import (
	"context"
	"slices"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"go.uber.org/fx"
)

var alwaysEnabled = []string{"account"}

func BuildApis(cm *config.Manager) fx.Option {
	var options []fx.Option
	enabledProtocols := cm.Viper().GetStringSlice("core.protocols")
	for _, entry := range registry.GetRegistry() {
		if slices.Contains(enabledProtocols, entry.Key) || slices.Contains(alwaysEnabled, entry.Key) {
			options = append(options, entry.Module)
		}
	}

	type initParams struct {
		fx.In
		Apis []registry.API `group:"api"`
	}

	options = append(options, fx.Invoke(func(params initParams) error {
		for _, protocol := range params.Apis {
			err := protocol.Init()
			if err != nil {
				return err
			}
		}

		return nil
	}))

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

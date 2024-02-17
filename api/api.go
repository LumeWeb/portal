package api

import (
	"context"
	"slices"

	"git.lumeweb.com/LumeWeb/portal/api/middleware"

	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func BuildApis(config *viper.Viper) fx.Option {
	var options []fx.Option
	enabledProtocols := config.GetStringSlice("core.protocols")
	for _, entry := range registry.GetRegistry() {
		if slices.Contains(enabledProtocols, entry.Key) {
			options = append(options, entry.Module)
		}
	}

	type initParams struct {
		fx.In
		Protocols []registry.API `group:"api"`
	}

	options = append(options, fx.Invoke(func(params initParams) error {
		for _, protocol := range params.Protocols {
			err := protocol.Init()
			if err != nil {
				return err
			}
		}

		return nil
	}))

	options = append(options, fx.Invoke(func(params initParams) error {
		for _, protocol := range params.Protocols {
			routes, err := protocol.Routes()
			if err != nil {
				return err
			}
			middleware.RegisterProtocolSubdomain(config, routes, protocol.Name())
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

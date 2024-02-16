package api

import (
	"context"
	"slices"

	"git.lumeweb.com/LumeWeb/portal/api/middleware"

	"git.lumeweb.com/LumeWeb/portal/api/account"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"git.lumeweb.com/LumeWeb/portal/api/s5"
	"github.com/spf13/viper"
	"go.uber.org/fx"
)

func RegisterApis() {
	registry.Register(registry.APIEntry{
		Key:    "s5",
		Module: s5.Module,
	})
	registry.Register(registry.APIEntry{
		Key:    "account",
		Module: account.Module,
	})
}

func BuildApis(config *viper.Viper) fx.Option {
	var options []fx.Option
	enabledProtocols := config.GetStringSlice("core.protocols")
	for _, entry := range registry.GetRegistry() {
		if slices.Contains(enabledProtocols, entry.Key) {
			options = append(options, entry.Module)
		}
	}

	options = append(options, fx.Invoke(func(protocols []registry.API) error {
		for _, protocol := range protocols {
			err := protocol.Init()
			if err != nil {
				return err
			}
		}

		return nil
	}))

	options = append(options, fx.Invoke(func(protocols []registry.API) {
		for _, protocol := range protocols {
			middleware.RegisterProtocolSubdomain(config, protocol.Routes(), protocol.Name())
		}
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

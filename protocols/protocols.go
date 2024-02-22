package protocols

import (
	"context"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/protocols/registry"
	"github.com/samber/lo"
	"go.uber.org/fx"
)

func BuildProtocols(cm *config.Manager) fx.Option {
	var options []fx.Option
	enabledProtocols := cm.Viper().GetStringSlice("core.protocols")
	for _, entry := range registry.GetRegistry() {
		if lo.Contains(enabledProtocols, entry.Key) {
			options = append(options, entry.Module)
			if entry.PreInitFunc != nil {
				options = append(options, fx.Invoke(entry.PreInitFunc))
			}
		}
	}

	type initParams struct {
		fx.In
		Protocols []registry.Protocol `group:"protocol"`
	}

	options = append(options, fx.Invoke(func(params initParams) error {
		for _, protocol := range params.Protocols {
			err := cm.ConfigureProtocol(protocol.Name(), protocol.Config())
			if err != nil {
				return err
			}

			err = protocol.Init()
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

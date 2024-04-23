package protocols

import (
	"context"

	"github.com/LumeWeb/portal/config"

	"github.com/LumeWeb/portal/protocols/registry"
	"github.com/samber/lo"
	"go.uber.org/fx"
)

func BuildProtocols(cm *config.Manager) fx.Option {
	var options []fx.Option
	enabledProtocols := cm.Viper().GetStringSlice("core.protocols")
	for _, entry := range registry.GetEntryRegistry() {
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
			registry.RegisterProtocol(protocol)
		}

		for _, protocol := range params.Protocols {
			err := cm.ConfigureProtocol(protocol.Name(), protocol.Config())
			if err != nil {
				return err
			}

			err = protocol.Init(context.Background())
			if err != nil {
				return err
			}
		}

		return nil
	}))

	return fx.Module("protocols", fx.Options(options...))
}

func SetupLifecycles(lifecycle fx.Lifecycle) error {
	for _, proto := range registry.GetAllProtocols() {
		lifecycle.Append(fx.Hook{
			OnStart: func(ctx context.Context) error {
				return proto.Start(ctx)
			},
			OnStop: func(ctx context.Context) error {
				return proto.Stop(ctx)
			},
		})
	}

	return nil
}

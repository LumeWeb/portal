package api

import (
	"context"
	"git.lumeweb.com/LumeWeb/portal/api/registry"
	"git.lumeweb.com/LumeWeb/portal/api/router"
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

func getModulesBasedOnConfig() []fx.Option {
	var modules []fx.Option
	for _, entry := range registry.GetRegistry() {
		if viper.GetBool("protocols." + entry.Key + ".enabled") {
			modules = append(modules, entry.Module)
		}
	}
	return modules
}

func BuildApis(config *viper.Viper) fx.Option {
	var options []fx.Option
	for _, entry := range registry.GetRegistry() {
		if config.GetBool("protocols." + entry.Key + ".enabled") {
			options = append(options, entry.Module)
			if entry.InitFunc != nil {
				options = append(options, fx.Invoke(entry.InitFunc))
			}
		}
	}

	return fx.Module("api", fx.Options(options...), fx.Provide(func() router.ProtocolRouter {
		return registry.GetRouter()
	}))
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

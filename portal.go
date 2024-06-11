package portal

import (
	"errors"
	"github.com/LumeWeb/portal/core"
	"github.com/LumeWeb/portal/db"
	"github.com/LumeWeb/portal/service"
	"go.uber.org/zap"
	"reflect"
	"sync"
)

var (
	activePortal                 Portal
	errInvalidServiceConstructor = errors.New("Invalid service constructor")
)

var services = []any{
	service.NewCronService,
	service.NewUserService,
	service.NewOTPService,
	service.NewAuthService,
	service.NewEmailVerificationService,
	service.NewPasswordResetService,
	service.NewImportService,
	func(ctx *core.Context) any {
		return service.NewMailerService(ctx, service.NewMailerTemplateRegistry())
	},
	service.NewRenterService,
	service.NewMetadataService,
	service.NewStorageService,
	service.NewPinService,
	service.NewSyncService,
	service.NewHTTPService,
}

type Portal interface {
	Init() error
	Start() error
	Stop() error
	Context() core.Context
	Serve() error
}

type PortalImpl struct {
	ctx   core.Context
	ctxMu sync.RWMutex
}

func (p *PortalImpl) Init() error {
	ctx := p.Context()

	dbInst := db.NewDatabase(&ctx)
	ctx.SetDB(dbInst)
	p.SetContext(ctx)

	instances := make([]any, 0)

	for _, svc := range services {
		svcVal := reflect.ValueOf(svc)
		if svcVal.Kind() != reflect.Func {
			ctx.Logger().Error("Invalid service constructor", zap.Any("constructor", svc))
			return errInvalidServiceConstructor
		}

		ctxType := reflect.TypeOf((*core.Context)(nil))
		if svcVal.Type().NumIn() != 1 || svcVal.Type().In(0) != ctxType {
			ctx.Logger().Error("Invalid service constructor signature", zap.Any("constructor", svc))
			return errInvalidServiceConstructor
		}

		ctxVal := reflect.ValueOf(&ctx)
		svcInstance := svcVal.Call([]reflect.Value{ctxVal})[0].Interface()

		ctx.RegisterService(svcInstance)
		instances = append(instances, svcInstance)
	}

	plugins := core.GetPlugins()

	for _, plugin := range plugins {
		if plugin.Models != nil && len(plugin.Models) > 0 {
			for _, model := range plugin.Models {
				typ := reflect.TypeOf(model)
				if typ.Kind() != reflect.Ptr {
					ctx.Logger().Error("Model must be a pointer", zap.String("model", typ.Name()))
					return core.ErrInvalidModel
				}
				if err := dbInst.AutoMigrate(model); err != nil {
					ctx.Logger().Error("Error migrating model", zap.String("model", typ.Name()), zap.Error(err))
					return err
				}
			}
		}
	}

	for _, plugin := range plugins {
		if core.PluginHasProtocol(plugin) {
			_proto, err := plugin.GetProtocol(&ctx)
			if err != nil {
				ctx.Logger().Error("Error building protocol", zap.String("plugin", plugin.ID), zap.Error(err))
				return err
			}

			if _proto == nil {
				continue
			}

			core.RegisterProtocol(plugin.ID, _proto)
		}
	}

	for _, plugin := range plugins {
		if core.PluginHasAPI(plugin) {
			api, err := plugin.GetAPI(&ctx)
			if err != nil {
				ctx.Logger().Error("Error building API", zap.String("plugin", plugin.ID), zap.Error(err))
				return err
			}

			if api == nil {
				continue
			}

			core.RegisterAPI(plugin.ID, api)
		}
	}

	for _, _proto := range core.GetProtocols() {
		err := ctx.Config().ConfigureProtocol(_proto.Name(), _proto.Config())
		if err != nil {
			ctx.Logger().Error("Error configuring protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
			return err
		}

		if initProto, ok := _proto.(core.ProtocolInit); ok {
			if err := initProto.Init(&ctx); err != nil {
				ctx.Logger().Error("Error initializing protocol", zap.String("protocol", _proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	for _, api := range core.GetAPIs() {
		if initApi, ok := api.(core.APIInit); ok {
			if err := initApi.Init(&ctx); err != nil {
				ctx.Logger().Error("Error initializing api", zap.String("api", api.Subdomain()), zap.Error(err))
				return err
			}
		}
	}

	p.SetContext(ctx)

	return nil
}

func (p *PortalImpl) Start() error {
	ctx := p.Context()
	ctx.Logger().Info("Starting portal")

	for _, startupFunc := range ctx.StartupFuncs() {
		if err := startupFunc(ctx); err != nil {
			ctx.Logger().Error("Error starting portal", zap.Error(err))
			return err
		}
	}

	for _, proto := range core.GetProtocols() {
		if startPlugin, ok := proto.(core.ProtocolStart); ok {
			if err := startPlugin.Start(ctx); err != nil {
				ctx.Logger().Error("Error starting protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func (p *PortalImpl) Stop() error {
	ctx := p.Context()
	ctx.Logger().Info("Stopping portal")
	for _, exitFunc := range ctx.ExitFuncs() {
		if err := exitFunc(ctx); err != nil {
			ctx.Logger().Error("Error stopping portal", zap.Error(err))
		}
	}

	for _, proto := range core.GetProtocols() {
		if stopPlugin, ok := proto.(core.ProtocolStop); ok {
			if err := stopPlugin.Stop(ctx); err != nil {
				ctx.Logger().Error("Error stopping protocol", zap.String("protocol", proto.Name()), zap.Error(err))
				return err
			}
		}
	}

	return nil
}

func (p *PortalImpl) Serve() error {
	ctx := p.Context()
	ctx.Logger().Info("Serving portal")
	return ctx.Services().HTTP().Serve()
}

func NewPortal(ctx core.Context) *PortalImpl {
	core.NewContext(ctx)

	return &PortalImpl{
		ctx: ctx,
	}
}

func (p *PortalImpl) Context() core.Context {
	p.ctxMu.RLock()
	defer p.ctxMu.RUnlock()
	return p.ctx
}

func (p *PortalImpl) SetContext(ctx core.Context) {
	p.ctxMu.Lock()
	defer p.ctxMu.Unlock()
	p.ctx = ctx
}

func NewActivePortal(ctx core.Context) {
	activePortal = NewPortal(ctx)
}

func Start() error {
	return activePortal.Start()
}

func Init() error {
	return activePortal.Init()
}

func Stop() error {
	return activePortal.Stop()
}

func Serve() error {
	return activePortal.Serve()
}

func Context() core.Context {
	return activePortal.Context()
}

func ActivePortal() Portal {
	return activePortal
}

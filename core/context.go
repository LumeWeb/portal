package core

import (
	"context"
	"github.com/LumeWeb/portal/config"
	"gorm.io/gorm"
)

type Context struct {
	context.Context
	services     Services
	cfg          config.Manager
	logger       *Logger
	exitFuncs    []func(Context) error
	startupFuncs []func(Context) error
	db           *gorm.DB
}

func (ctx *Context) Services() Services {
	return ctx.services
}

type Services struct {
	auth       AuthService
	user       UserService
	userVerify EmailVerificationService
	dnslink    DNSLinkService
	pin        PinService
	password   PasswordResetService
	cron       CronService
	importer   ImportService
	mailer     MailerService
	metadata   MetadataService
	storage    StorageService
	otp        OTPService
	renter     RenterService
	sync       SyncService
	http       HTTPService
}

func NewBaseContext(config config.Manager, logger *Logger) Context {
	return Context{Context: context.Background(), cfg: config, logger: logger}
}

func NewContext(ctx Context) (Context, context.CancelFunc) {
	newCtx := Context{cfg: ctx.cfg, logger: ctx.logger}
	c, cancel := context.WithCancel(ctx)

	wrappedCancel := func() {
		cancel()
	}

	newCtx.Context = c
	return newCtx, wrappedCancel
}
func (ctx *Context) OnExit(f func(Context) error) {
	ctx.exitFuncs = append(ctx.exitFuncs, f)
}

func (ctx *Context) OnStartup(f func(Context) error) {
	ctx.startupFuncs = append(ctx.startupFuncs, f)
}

func (ctx *Context) StartupFuncs() []func(Context) error {
	return ctx.startupFuncs
}

func (ctx *Context) ExitFuncs() []func(Context) error {
	return ctx.exitFuncs
}

func (ctx *Context) RegisterService(svc any) {
	switch svc := svc.(type) {
	case AuthService:
		ctx.services.auth = svc
	case UserService:
		ctx.services.user = svc
	case EmailVerificationService:
		ctx.services.userVerify = svc
	case DNSLinkService:
		ctx.services.dnslink = svc
	case PinService:
		ctx.services.pin = svc
	case PasswordResetService:
		ctx.services.password = svc
	case CronService:
		ctx.services.cron = svc
	case ImportService:
		ctx.services.importer = svc
	case MailerService:
		ctx.services.mailer = svc
	case MetadataService:
		ctx.services.metadata = svc
	case StorageService:
		ctx.services.storage = svc
	case OTPService:
		ctx.services.otp = svc
	case RenterService:
		ctx.services.renter = svc
	case SyncService:
		ctx.services.sync = svc
	case HTTPService:
		ctx.services.http = svc
	}
}

func (ctx *Context) SetDB(db *gorm.DB) {
	ctx.db = db
}

func (ctx *Context) DB() *gorm.DB {
	return ctx.db
}

func (ctx *Context) Logger() *Logger {
	return ctx.logger
}

func (ctx *Context) Config() config.Manager {
	return ctx.cfg
}

func (s Services) Auth() AuthService {
	return s.auth
}

func (s Services) User() UserService {
	return s.user
}

func (s Services) UserVerify() EmailVerificationService {
	return s.userVerify
}

func (s Services) DNSLink() DNSLinkService {
	return s.dnslink
}

func (s Services) Pin() PinService {
	return s.pin
}

func (s Services) Password() PasswordResetService {
	return s.password
}

func (s Services) Cron() CronService {
	return s.cron
}

func (s Services) Importer() ImportService {
	return s.importer
}

func (s Services) Mailer() MailerService {
	return s.mailer
}

func (s Services) Metadata() MetadataService {
	return s.metadata
}

func (s Services) Storage() StorageService {
	return s.storage
}

func (s Services) Otp() OTPService {
	return s.otp
}

func (s Services) Renter() RenterService {
	return s.renter
}

func (s Services) Sync() SyncService {
	return s.sync
}

func (s Services) HTTP() HTTPService {
	return s.http
}

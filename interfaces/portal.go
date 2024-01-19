package interfaces

import (
	"crypto/ed25519"
	"github.com/casbin/casbin/v2"
	"github.com/go-co-op/gocron/v2"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Portal interface {
	Initialize() error
	Run()
	Config() *viper.Viper
	Logger() *zap.Logger
	ApiRegistry() APIRegistry
	ProtocolRegistry() ProtocolRegistry
	Identity() ed25519.PrivateKey
	Storage() StorageService
	SetIdentity(identity ed25519.PrivateKey)
	SetLogger(logger *zap.Logger)
	Database() *gorm.DB
	DatabaseService() Database
	Casbin() *casbin.Enforcer
	SetCasbin(e *casbin.Enforcer)
	Accounts() AccountService
	CronService() CronService
	Cron() gocron.Scheduler
}

package interfaces

import (
	"crypto/ed25519"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type Portal interface {
	Initialize() error
	Run()
	Config() *viper.Viper
	Logger() *zap.Logger
	Db() *gorm.DB
	ApiRegistry() APIRegistry
	Identity() ed25519.PrivateKey
}

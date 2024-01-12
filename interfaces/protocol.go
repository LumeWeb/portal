package interfaces

import (
	"github.com/spf13/viper"
	"go.uber.org/zap"
)

type Protocol interface {
	Initialize(config *viper.Viper, logger *zap.Logger) error
	Start() error
}

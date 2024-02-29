package db

import (
	"context"
	"fmt"

	"git.lumeweb.com/LumeWeb/portal/db/models"

	"github.com/redis/go-redis/v9"

	"go.uber.org/zap"

	"git.lumeweb.com/LumeWeb/portal/config"

	"github.com/go-gorm/caches/v4"
	"go.uber.org/fx"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type DatabaseParams struct {
	fx.In
	Config      *config.Manager
	Logger      *zap.Logger
	LoggerLevel *zap.AtomicLevel
}

var Module = fx.Module("db",
	fx.Options(
		fx.Provide(NewDatabase),
	),
)

func NewDatabase(lc fx.Lifecycle, params DatabaseParams) *gorm.DB {
	username := params.Config.Config().Core.DB.Username
	password := params.Config.Config().Core.DB.Password
	host := params.Config.Config().Core.DB.Host
	port := params.Config.Config().Core.DB.Port
	dbname := params.Config.Config().Core.DB.Name
	charset := params.Config.Config().Core.DB.Charset

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=%s&parseTime=True&loc=Local", username, password, host, port, dbname, charset)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: newLogger(params.Logger, params.LoggerLevel),
	})
	if err != nil {
		panic(err)
	}

	cacher := getCacher(params.Config, params.Logger)
	if cacher != nil {
		cache := &caches.Caches{Conf: &caches.Config{
			Cacher: cacher,
		}}
		err := db.Use(cache)
		if err != nil {
			return nil
		}
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return db.AutoMigrate(models.GetModels()...)
		},
	})

	return db
}

func getCacheMode(cm *config.Manager, logger *zap.Logger) string {

	if cm.Config().Core.DB.Cache == nil {
		return "none"
	}

	switch cm.Config().Core.DB.Cache.Mode {
	case "", "none":
		return "none"
	case "memory":
		return "memory"
	case "redis":
		return "redis"
	default:
		logger.Fatal("invalid cache mode", zap.String("mode", cm.Config().Core.DB.Cache.Mode))
	}

	return "none"
}

func getCacher(cm *config.Manager, logger *zap.Logger) caches.Cacher {
	mode := getCacheMode(cm, logger)

	switch mode {
	case "none":
		return nil

	case "memory":
		return &memoryCacher{}
	case "redis":
		rcfg, ok := cm.Config().Core.DB.Cache.Options.(config.RedisConfig)
		if !ok {
			logger.Fatal("invalid redis config")
			return nil
		}
		return &redisCacher{
			redis.NewClient(&redis.Options{
				Addr:     rcfg.Address,
				Password: rcfg.Password,
				DB:       rcfg.DB,
			}),
		}
	}

	return nil
}

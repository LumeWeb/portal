package db

import (
	"context"
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"github.com/spf13/viper"
	"go.uber.org/fx"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type DatabaseParams struct {
	fx.In
	Config *viper.Viper
}

var Module = fx.Module("db",
	fx.Options(
		fx.Provide(NewDatabase),
	),
)

func NewDatabase(lc fx.Lifecycle, params DatabaseParams) *gorm.DB {
	username := params.Config.GetString("core.db.username")
	password := params.Config.GetString("core.db.password")
	host := params.Config.GetString("core.db.host")
	port := params.Config.GetString("core.db.port")
	dbname := params.Config.GetString("core.db.name")
	charset := params.Config.GetString("core.db.charset")

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=True&loc=Local", username, password, host, port, dbname, charset)

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		panic(err)
	}

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return db.AutoMigrate(
				&models.APIKey{},
				&models.Blocklist{},
				&models.Download{},
				&models.Pin{},
				&models.PublicKey{},
				&models.Upload{},
				&models.User{},
				&models.S5Challenge{},
				&models.TusLock{},
				&models.TusUpload{},
			)
		},
	})

	return db
}

package db

import (
	"context"
	"fmt"

	"git.lumeweb.com/LumeWeb/portal/config"

	"git.lumeweb.com/LumeWeb/portal/db/models"
	"go.uber.org/fx"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type DatabaseParams struct {
	fx.In
	Config *config.Manager
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

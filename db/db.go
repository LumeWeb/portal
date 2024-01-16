package db

import (
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/db/models"
	"git.lumeweb.com/LumeWeb/portal/interfaces"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var (
	_ interfaces.Database = (*DatabaseImpl)(nil)
)

type DatabaseImpl struct {
	db     *gorm.DB
	portal interfaces.Portal
}

func NewDatabase(p interfaces.Portal) interfaces.Database {
	return &DatabaseImpl{
		portal: p,
	}
}

// Init initializes the database connection
func (d *DatabaseImpl) Init(p interfaces.Portal) error {
	// Retrieve DB config from Viper
	username := viper.GetString("core.db.username")
	password := viper.GetString("core.db.password")
	host := viper.GetString("core.db.host")
	port := viper.GetString("core.db.port")
	dbname := viper.GetString("core.db.name")
	charset := viper.GetString("core.db.charset")

	// Construct DSN
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=%s&parseTime=True&loc=Local", username, password, host, port, dbname, charset)

	// Open DB connection
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		p.Logger().Error("Failed to connect to database", zap.Error(err))
	}
	d.db = db

	return nil
}

// Start performs any additional setup
func (d *DatabaseImpl) Start() error {
	return d.db.AutoMigrate(
		&models.APIKey{},
		&models.Blocklist{},
		&models.Download{},
		&models.Pin{},
		&models.PublicKey{},
		&models.Upload{},
		&models.User{},
		&models.S5Challenge{},
	)
}

func (d *DatabaseImpl) Get() *gorm.DB {
	return d.db
}

package db

import (
	"fmt"
	"git.lumeweb.com/LumeWeb/portal/model"
	"github.com/spf13/viper"
	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// Declare a global variable to hold the database connection.
var db *gorm.DB

// Init initializes the database connection based on the app's configuration settings.
func Init() {
	// If the database connection has already been initialized, panic.
	if db != nil {
		panic("DB already initialized")
	}

	// Retrieve database connection settings from the app's configuration using the viper library.
	dbType := viper.GetString("database.type")
	dbHost := viper.GetString("database.host")
	dbPort := viper.GetInt("database.port")
	dbSocket := viper.GetString("database.socket")
	dbUser := viper.GetString("database.user")
	dbPassword := viper.GetString("database.password")
	dbName := viper.GetString("database.name")
	dbPath := viper.GetString("database.path")

	var err error
	var dsn string
	switch dbType {
	// Connect to a MySQL database.
	case "mysql":
		if dbSocket != "" {
			dsn = fmt.Sprintf("%s:%s@unix(%s)/%s", dbUser, dbPassword, dbSocket, dbName)
		} else {
			dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s", dbUser, dbPassword, dbHost, dbPort, dbName)
		}
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	// Connect to a SQLite database.
	case "sqlite":
		db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	// If the database type is unsupported, panic.
	default:
		panic(fmt.Errorf("Unsupported database type: %s \n", dbType))
	}
	// If there was an error connecting to the database, panic.
	if err != nil {
		panic(fmt.Errorf("Failed to connect to database: %s \n", err))
	}

	// Automatically migrate the database schema based on the model definitions.
	err = db.Migrator().AutoMigrate(&model.Account{}, &model.Key{}, &model.KeyChallenge{}, &model.LoginSession{})
	if err != nil {
		panic(fmt.Errorf("Database setup failed database type: %s \n", err))
	}
}

// Get returns the database connection instance.
func Get() *gorm.DB {
	return db
}

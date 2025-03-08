package core

import "io/fs"

type DBType string

const (
	DB_TYPE_MYSQL  DBType = "mysql"
	DB_TYPE_SQLITE DBType = "sqlite"
)

type DBMigration map[DBType]fs.FS

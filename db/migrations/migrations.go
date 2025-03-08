package migrations

import (
	"embed"
	"io/fs"
)

//go:embed mysql/*.sql
var mysql embed.FS

//go:embed sqlite/*.sql
var sqlite embed.FS

func GetMySQL() fs.FS {
	sub, err := fs.Sub(mysql, "mysql")
	if err != nil {
		panic(err)
	}

	return sub
}

func GetSQLite() fs.FS {
	sub, err := fs.Sub(sqlite, "sqlite")
	if err != nil {
		panic(err)
	}
	return sub
}

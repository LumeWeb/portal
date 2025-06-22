package migrations

import (
	"embed"
	_ "go.lumeweb.com/portal/core/internal/core_tests/fixtures/migrations/sqlite"
	"io/fs"
)

//go:embed sqlite/*
var sqlite embed.FS

func GetSQLite() fs.FS {
	sub, err := fs.Sub(sqlite, "sqlite")
	if err != nil {
		panic(err)
	}
	return sub
}

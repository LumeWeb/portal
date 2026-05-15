package migrations

import (
	"context"
	"database/sql"
	"github.com/pressly/goose/v3"
)

func init() {
	goose.AddMigrationContext(upInit, downInit)
}

func upInit(_ context.Context, tx *sql.Tx) error {
	// This code is executed when the migration is applied.
	_, err := tx.Exec(`
-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS test_table (id INTEGER PRIMARY KEY);
-- +goose StatementEnd
`)
	if err != nil {
		return err
	}
	return nil
}

func downInit(_ context.Context, tx *sql.Tx) error {
	// This code is executed when the migration is rolled back.
	_, err := tx.Exec(`
-- +goose Down
-- +goose StatementBegin
DROP TABLE test_table;
-- +goose StatementEnd
`)
	if err != nil {
		return err
	}
	return nil
}

-- +goose Up

-- Create object_sync_cursor table (stores the sealed-data refresh loop cursor)
CREATE TABLE IF NOT EXISTS `object_sync_cursor`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `cursor`     text
);

-- +goose Down

DROP TABLE IF EXISTS `object_sync_cursor`;

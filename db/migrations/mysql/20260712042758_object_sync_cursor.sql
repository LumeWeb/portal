-- +goose Up

-- Create object_sync_cursor table (stores the sealed-data refresh loop cursor)
CREATE TABLE IF NOT EXISTS `object_sync_cursor`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3) DEFAULT NULL,
    `updated_at` datetime(3) DEFAULT NULL,
    `deleted_at` datetime(3) DEFAULT NULL,
    `cursor`     longtext,
    PRIMARY KEY (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- +goose Down

DROP TABLE IF EXISTS `object_sync_cursor`;

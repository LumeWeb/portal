-- +goose Up

-- Create renter_objects table (tracks Sia object lifecycle: staged → packing → uploaded → deleting)
-- Named renter_objects to avoid collision with the Sia plugin's sia_objects table.
CREATE TABLE IF NOT EXISTS `renter_objects`
(
    `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`    datetime(3) DEFAULT NULL,
    `updated_at`    datetime(3) DEFAULT NULL,
    `deleted_at`    datetime(3) DEFAULT NULL,
    `protocol`      varchar(191) DEFAULT NULL,
    `bucket`        varchar(191) DEFAULT NULL,
    `object_key`    varchar(191) DEFAULT NULL,
    `hash`          longblob,
    `size`          bigint,
    `sia_object_id` varchar(191) DEFAULT NULL,
    `staging_key`   varchar(191) DEFAULT NULL,
    `sealed_data`   JSON,
    `status`        varchar(191) NOT NULL DEFAULT 'staged',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_renter_object_key` (`protocol`, `object_key`),
    KEY `idx_renter_objects_deleted_at` (`deleted_at`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- Drop the old sia_uploads table (replaced by renter_objects)
DROP TABLE IF EXISTS `sia_uploads`;

-- +goose Down

-- Note: sia_uploads was dropped in Up and is not recreated here.
-- This migration is one-way; rollback requires a backup/restore of sia_uploads data.
DROP TABLE IF EXISTS `renter_objects`;

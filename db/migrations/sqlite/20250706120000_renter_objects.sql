-- +goose Up

-- Create renter_objects table (tracks Sia object lifecycle: staged → packing → uploaded → deleting)
-- Named renter_objects to avoid collision with the Sia plugin's sia_objects table.
CREATE TABLE IF NOT EXISTS `renter_objects`
(
    `id`            integer PRIMARY KEY AUTOINCREMENT,
    `created_at`    datetime,
    `updated_at`    datetime,
    `deleted_at`    datetime,
    `protocol`      text,
    `bucket`        text,
    `object_key`    text,
    `size`          integer,
    `sia_object_id` text,
    `staging_key`   text,
    `sealed_data`   JSON,
    `status`        text NOT NULL DEFAULT 'staged',
    CONSTRAINT `idx_renter_object_key` UNIQUE (`protocol`, `object_key`)
);
CREATE INDEX IF NOT EXISTS `idx_renter_objects_deleted_at` ON `renter_objects` (`deleted_at`);

-- Drop the old sia_uploads table (replaced by renter_objects)
DROP TABLE IF EXISTS `sia_uploads`;

-- +goose Down

-- Note: sia_uploads was dropped in Up and is not recreated here.
-- This migration is one-way; rollback requires a backup/restore of sia_uploads data.
DROP TABLE IF EXISTS `renter_objects`;

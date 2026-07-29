-- +goose Up
-- Composite index to optimize queries filtering uploads by protocol + mime_type.
-- Primarily benefits ProtocolStorageStatsProvider implementations (e.g. Sia plugin's
-- StorageStats) which filter on protocol, mime_type, and deleted_at to distinguish
-- slab pins from virtual object pins.
CREATE INDEX `idx_uploads_protocol_mime_deleted` ON `uploads` (`protocol`(255), `mime_type`(255), `deleted_at`);

-- +goose Down
DROP INDEX `idx_uploads_protocol_mime_deleted` ON `uploads`;

-- +goose Up

-- SQLite does not support ALTER TABLE RENAME with column adds well.
-- Strategy: create new table with correct name and schema, copy data, drop old.

CREATE TABLE IF NOT EXISTS `key_identities` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `user_id` integer NOT NULL,
    `type` TEXT NOT NULL DEFAULT 'ethereum',
    `key` TEXT NOT NULL,
    `metadata` TEXT,
    CONSTRAINT `fk_users_key_identities` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`),
    UNIQUE (`type`, `key`)
);

INSERT INTO `key_identities` (`id`, `created_at`, `updated_at`, `deleted_at`, `user_id`, `type`, `key`, `metadata`)
SELECT `id`, `created_at`, `updated_at`, `deleted_at`, `user_id`, 'ethereum', `key`, NULL FROM `public_keys`;

DROP TABLE `public_keys`;

CREATE INDEX IF NOT EXISTS `idx_key_identities_type` ON `key_identities` (`type`);
CREATE INDEX IF NOT EXISTS `idx_key_identities_user_id` ON `key_identities` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_key_identities_deleted_at` ON `key_identities` (`deleted_at`);

-- +goose Down

CREATE TABLE IF NOT EXISTS `public_keys` (
    `id` integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `user_id` integer NOT NULL,
    `key` TEXT NOT NULL,
    CONSTRAINT `fk_users_public_keys` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`),
    CONSTRAINT `uni_public_keys_key` UNIQUE (`key`)
);

-- Insert rows from key_identities back into public_keys.
-- The UNIQUE(key) constraint above will cause this INSERT to fail with a
-- clear error if duplicate key strings exist across different types
-- (valid under UNIQUE(type, key) but not under UNIQUE(key)).
INSERT INTO `public_keys` (`id`, `created_at`, `updated_at`, `deleted_at`, `user_id`, `key`)
SELECT `id`, `created_at`, `updated_at`, `deleted_at`, `user_id`, `key` FROM `key_identities`;

DROP TABLE `key_identities`;

CREATE INDEX IF NOT EXISTS `idx_public_keys_user_id` ON `public_keys` (`user_id`);
CREATE INDEX IF NOT EXISTS `idx_public_keys_deleted_at` ON `public_keys` (`deleted_at`);

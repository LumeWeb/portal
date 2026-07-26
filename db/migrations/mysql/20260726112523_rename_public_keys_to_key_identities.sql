-- +goose Up

-- Rename table
RENAME TABLE `public_keys` TO `key_identities`;

-- Add type and metadata columns
-- Using ALGORITHM=INPLACE and LOCK=NONE to avoid blocking reads/writes
ALTER TABLE `key_identities`
    ADD COLUMN `type` VARCHAR(50) NOT NULL DEFAULT 'ethereum' AFTER `user_id`,
    ADD COLUMN `metadata` JSON NULL AFTER `key`,
    ALGORITHM=INPLACE, LOCK=NONE;

-- Drop old unique index on `key` alone, add composite unique on (type, key)
SET @idx_exists = (SELECT COUNT(*) FROM information_schema.STATISTICS WHERE table_schema = DATABASE() AND table_name = 'key_identities' AND index_name = 'uni_public_keys_key');
SET @sql = IF(@idx_exists > 0, 'ALTER TABLE `key_identities` DROP INDEX `uni_public_keys_key`', 'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

ALTER TABLE `key_identities` ADD UNIQUE INDEX `uk_key_identities_type_key` (`type`, `key`);
CREATE INDEX `idx_key_identities_type` ON `key_identities` (`type`);

-- +goose Down

ALTER TABLE `key_identities` DROP INDEX `uk_key_identities_type_key`;
ALTER TABLE `key_identities` DROP INDEX `idx_key_identities_type`;
ALTER TABLE `key_identities` DROP COLUMN `metadata`;
ALTER TABLE `key_identities` DROP COLUMN `type`;

RENAME TABLE `key_identities` TO `public_keys`;

-- Detect duplicate key strings that would collide under a single-column unique index
SET @dupes = (SELECT COUNT(*) FROM (SELECT `key` FROM `public_keys` GROUP BY `key` HAVING COUNT(*) > 1) AS dups LIMIT 1);
SET @sql = IF(@dupes > 0,
    'SIGNAL SQLSTATE ''45000'' SET MESSAGE_TEXT = ''Duplicate key strings found; cannot restore single-column unique index''',
    'SELECT 1');
PREPARE stmt FROM @sql;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

ALTER TABLE `public_keys` ADD UNIQUE INDEX `uni_public_keys_key` (`key`);

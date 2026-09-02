-- +goose Up

CREATE TABLE `social_accounts`
(
    `id`                integer PRIMARY KEY AUTOINCREMENT,
    `created_at`        datetime,
    `updated_at`        datetime,
    `deleted_at`        datetime,
    `user_id`           integer NOT NULL,
    `provider`          text NOT NULL,
    `provider_user_id`  text NOT NULL,
    `email`             text,
    CONSTRAINT `uk_social_accounts_provider_uid` UNIQUE (`provider`, `provider_user_id`)
);

CREATE INDEX `idx_social_accounts_user_id` ON `social_accounts` (`user_id`);
CREATE INDEX `idx_social_accounts_provider` ON `social_accounts` (`provider`);
CREATE INDEX `idx_social_accounts_deleted_at` ON `social_accounts` (`deleted_at`);

-- +goose Down
DROP TABLE `social_accounts`;

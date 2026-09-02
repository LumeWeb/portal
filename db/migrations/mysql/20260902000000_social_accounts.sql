-- +goose Up

CREATE TABLE `social_accounts`
(
    `id`                bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`        datetime(3) DEFAULT NULL,
    `updated_at`        datetime(3) DEFAULT NULL,
    `deleted_at`        datetime(3) DEFAULT NULL,
    `user_id`           bigint unsigned NOT NULL,
    `provider`          varchar(50) NOT NULL,
    `provider_user_id`  varchar(255) NOT NULL,
    `email`             varchar(255) DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_social_accounts_provider_uid` (`provider`, `provider_user_id`),
    KEY `idx_social_accounts_user_id` (`user_id`),
    KEY `idx_social_accounts_provider` (`provider`),
    KEY `idx_social_accounts_deleted_at` (`deleted_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE `social_accounts`;

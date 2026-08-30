-- +goose Up

CREATE TABLE `oauth_clients`
(
    `id`                        bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`                datetime(3) DEFAULT NULL,
    `updated_at`                datetime(3) DEFAULT NULL,
    `deleted_at`                datetime(3) DEFAULT NULL,
    `client_id`                 varchar(191) NOT NULL,
    `client_name`               longtext,
    `redirect_uris`             longtext,
    `grant_types`               longtext,
    `response_types`            longtext,
    `token_endpoint_auth_method` longtext,
    `scopes`                    longtext,
    `user_id`                   bigint unsigned DEFAULT NULL,
    `is_active`                 tinyint(1) DEFAULT '1',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_oauth_clients_client_id` (`client_id`),
    KEY `idx_oauth_clients_user_id` (`user_id`),
    KEY `idx_oauth_clients_deleted_at` (`deleted_at`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE `oauth_authorization_codes`
(
    `id`                     bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`             datetime(3) DEFAULT NULL,
    `updated_at`             datetime(3) DEFAULT NULL,
    `deleted_at`             datetime(3) DEFAULT NULL,
    `code`                   varchar(191) NOT NULL,
    `client_id`              varchar(191) NOT NULL,
    `redirect_uri`           longtext,
    `code_challenge`         longtext,
    `code_challenge_method`  longtext,
    `resource`               longtext,
    `user_id`                bigint unsigned NOT NULL,
    `scope`                  longtext,
    `expires_at`             datetime(3) NOT NULL,
    `used_at`                datetime(3) DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_oauth_authorization_codes_code` (`code`),
    KEY `idx_oauth_authorization_codes_client_id` (`client_id`),
    KEY `idx_oauth_authorization_codes_deleted_at` (`deleted_at`),
    CONSTRAINT `fk_oauth_authorization_codes_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE `oauth_refresh_tokens`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`  datetime(3) DEFAULT NULL,
    `updated_at`  datetime(3) DEFAULT NULL,
    `deleted_at`  datetime(3) DEFAULT NULL,
    `token`       varchar(191) NOT NULL,
    `client_id`   varchar(191) NOT NULL,
    `resource`    longtext,
    `user_id`     bigint unsigned NOT NULL,
    `chain_root`  varchar(255) DEFAULT NULL,
    `expires_at`  datetime(3) NOT NULL,
    `used_at`     datetime(3) DEFAULT NULL,
    `revoked`     tinyint(1) DEFAULT '0',
    `successor`   varchar(191) DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_oauth_refresh_tokens_token` (`token`),
    KEY `idx_oauth_refresh_tokens_client_id` (`client_id`),
    KEY `idx_oauth_refresh_tokens_chain_root` (`chain_root`),
    KEY `idx_oauth_refresh_tokens_deleted_at` (`deleted_at`),
    CONSTRAINT `fk_oauth_refresh_tokens_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

CREATE TABLE `oauth_access_tokens`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`  datetime(3) DEFAULT NULL,
    `updated_at`  datetime(3) DEFAULT NULL,
    `deleted_at`  datetime(3) DEFAULT NULL,
    `token`       varchar(191) NOT NULL,
    `client_id`   varchar(191) NOT NULL,
    `resource`    longtext,
    `user_id`     bigint unsigned NOT NULL,
    `expires_at`  datetime(3) NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_oauth_access_tokens_token` (`token`),
    KEY `idx_oauth_access_tokens_client_id` (`client_id`),
    KEY `idx_oauth_access_tokens_deleted_at` (`deleted_at`),
    CONSTRAINT `fk_oauth_access_tokens_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB DEFAULT CHARSET = utf8mb4 COLLATE = utf8mb4_0900_ai_ci;

-- +goose Down
DROP TABLE `oauth_access_tokens`;
DROP TABLE `oauth_refresh_tokens`;
DROP TABLE `oauth_authorization_codes`;
DROP TABLE `oauth_clients`;

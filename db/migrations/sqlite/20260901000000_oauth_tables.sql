-- +goose Up

CREATE TABLE `oauth_clients`
(
    `id`                        integer PRIMARY KEY AUTOINCREMENT,
    `created_at`                datetime,
    `updated_at`                datetime,
    `deleted_at`                datetime,
    `client_id`                 text NOT NULL,
    `client_name`               text,
    `redirect_uris`             text,
    `grant_types`               text,
    `response_types`            text,
    `token_endpoint_auth_method` text,
    `scopes`                    text,
    `user_id`                   integer,
    `is_active`                 numeric DEFAULT 1,
    CONSTRAINT `uni_oauth_clients_client_id` UNIQUE (`client_id`)
);
CREATE INDEX `idx_oauth_clients_user_id` ON `oauth_clients` (`user_id`);
CREATE INDEX `idx_oauth_clients_deleted_at` ON `oauth_clients` (`deleted_at`);

CREATE TABLE `oauth_authorization_codes`
(
    `id`                     integer PRIMARY KEY AUTOINCREMENT,
    `created_at`             datetime,
    `updated_at`             datetime,
    `deleted_at`             datetime,
    `code`                   text NOT NULL,
    `client_id`              text NOT NULL,
    `redirect_uri`           text,
    `code_challenge`         text,
    `code_challenge_method`  text,
    `resource`               text,
    `user_id`                integer NOT NULL,
    `scope`                  text,
    `expires_at`             datetime NOT NULL,
    `used_at`                datetime,
    CONSTRAINT `uni_oauth_authorization_codes_code` UNIQUE (`code`),
    CONSTRAINT `fk_oauth_authorization_codes_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_oauth_authorization_codes_client_id` ON `oauth_authorization_codes` (`client_id`);
CREATE INDEX `idx_oauth_authorization_codes_deleted_at` ON `oauth_authorization_codes` (`deleted_at`);

CREATE TABLE `oauth_refresh_tokens`
(
    `id`          integer PRIMARY KEY AUTOINCREMENT,
    `created_at`  datetime,
    `updated_at`  datetime,
    `deleted_at`  datetime,
    `token`       text NOT NULL,
    `client_id`   text NOT NULL,
    `resource`    text,
    `user_id`     integer NOT NULL,
    `scope`       text,
    `chain_root`  text,
    `expires_at`  datetime NOT NULL,
    `used_at`     datetime,
    `revoked`     numeric DEFAULT 0,
    `successor`   text,
    CONSTRAINT `uni_oauth_refresh_tokens_token` UNIQUE (`token`),
    CONSTRAINT `fk_oauth_refresh_tokens_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_oauth_refresh_tokens_client_id` ON `oauth_refresh_tokens` (`client_id`);
CREATE INDEX `idx_oauth_refresh_tokens_chain_root` ON `oauth_refresh_tokens` (`chain_root`);
CREATE INDEX `idx_oauth_refresh_tokens_deleted_at` ON `oauth_refresh_tokens` (`deleted_at`);

CREATE TABLE `oauth_access_tokens`
(
    `id`          integer PRIMARY KEY AUTOINCREMENT,
    `created_at`  datetime,
    `updated_at`  datetime,
    `deleted_at`  datetime,
    `token`       text NOT NULL,
    `client_id`   text NOT NULL,
    `resource`    text,
    `user_id`     integer NOT NULL,
    `scope`       text,
    `expires_at`  datetime NOT NULL,
    CONSTRAINT `uni_oauth_access_tokens_token` UNIQUE (`token`),
    CONSTRAINT `fk_oauth_access_tokens_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_oauth_access_tokens_client_id` ON `oauth_access_tokens` (`client_id`);
CREATE INDEX `idx_oauth_access_tokens_deleted_at` ON `oauth_access_tokens` (`deleted_at`);

-- +goose Down
DROP TABLE `oauth_access_tokens`;
DROP TABLE `oauth_refresh_tokens`;
DROP TABLE `oauth_authorization_codes`;
DROP TABLE `oauth_clients`;

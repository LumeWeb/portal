-- +goose Up

-- 1. Independent tables (no foreign key dependencies)
CREATE TABLE `access_rules`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `ptype`      text,
    `v0`         text,
    `v1`         text,
    `v2`         text,
    `v3`         text,
    `v4`         text,
    `v5`         text
);
CREATE UNIQUE INDEX `idx_access_rule` ON `access_rules` (`ptype`, `v0`, `v1`, `v2`, `v3`, `v4`, `v5`);
CREATE INDEX `idx_access_rules_deleted_at` ON `access_rules` (`deleted_at`);

CREATE TABLE `hash_mappings`
(
    `id`          integer PRIMARY KEY AUTOINCREMENT,
    `created_at`  datetime,
    `updated_at`  datetime,
    `deleted_at`  datetime,
    `source_hash` varbinary(64),
    `target_hash` varbinary(64),
    `protocol`    varchar(255),
    `metadata`    JSON
);
CREATE INDEX `idx_hash_mapping_protocol` ON `hash_mappings` (`protocol`);
CREATE INDEX `idx_hash_mapping_target` ON `hash_mappings` (`target_hash`);
CREATE INDEX `idx_hash_mapping_source` ON `hash_mappings` (`source_hash`);
CREATE INDEX `idx_hash_mappings_deleted_at` ON `hash_mappings` (`deleted_at`);

CREATE TABLE `s3_uploads`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `upload_id`  text NOT NULL,
    `bucket`     text NOT NULL,
    `key`        text NOT NULL,
    CONSTRAINT `uni_s3_uploads_upload_id` UNIQUE (`upload_id`)
);
CREATE INDEX `idx_s3_bucket_key` ON `s3_uploads` (`bucket`, `key`);
CREATE INDEX `idx_s3_uploads_deleted_at` ON `s3_uploads` (`deleted_at`);

CREATE TABLE `scan_results`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `hash`       varbinary(64),
    `scanner_id` text,
    `passed`     numeric,
    `reason`     text,
    `metadata`   JSON
);
CREATE INDEX `idx_scan_results_scanner_id` ON `scan_results` (`scanner_id`);
CREATE INDEX `idx_scan_results_hash` ON `scan_results` (`hash`);
CREATE INDEX `idx_scan_results_deleted_at` ON `scan_results` (`deleted_at`);

CREATE TABLE `sia_uploads`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `upload_id`  text NOT NULL,
    `bucket`     text NOT NULL,
    `key`        text NOT NULL,
    CONSTRAINT `uni_sia_uploads_upload_id` UNIQUE (`upload_id`)
);
CREATE INDEX `idx_sia_bucket_key` ON `sia_uploads` (`bucket`, `key`);
CREATE INDEX `idx_sia_uploads_deleted_at` ON `sia_uploads` (`deleted_at`);

CREATE TABLE `tus_locks`
(
    `id`                integer PRIMARY KEY AUTOINCREMENT,
    `created_at`        datetime,
    `updated_at`        datetime,
    `deleted_at`        datetime,
    `lock_id`           text,
    `holder_p_id`       integer,
    `acquired_at`       datetime,
    `expires_at`        datetime,
    `release_requested` numeric
);
CREATE INDEX `idx_tus_locks_holder_p_id` ON `tus_locks` (`holder_p_id`);
CREATE UNIQUE INDEX `idx_lock_id` ON `tus_locks` (`lock_id`, `deleted_at`);
CREATE INDEX `idx_tus_locks_deleted_at` ON `tus_locks` (`deleted_at`);

-- 2. Primary Tables - These are referenced by other tables
CREATE TABLE `users`
(
    `id`            integer PRIMARY KEY AUTOINCREMENT,
    `created_at`    datetime,
    `updated_at`    datetime,
    `deleted_at`    datetime,
    `first_name`    text,
    `last_name`     text,
    `email`         text,
    `password_hash` text,
    `role`          text,
    `last_login`    datetime,
    `last_login_ip` text,
    `otp_enabled`   numeric DEFAULT false,
    `otp_verified`  numeric DEFAULT false,
    `otp_secret`    text,
    `otp_auth_url`  text,
    `verified`      numeric DEFAULT false,
    CONSTRAINT `uni_users_email` UNIQUE (`email`)
);
CREATE INDEX `idx_users_deleted_at` ON `users` (`deleted_at`);

CREATE TABLE `cron_jobs`
(
    `id`             integer PRIMARY KEY AUTOINCREMENT,
    `created_at`     datetime,
    `updated_at`     datetime,
    `deleted_at`     datetime,
    `uuid`           binary(16),
    `origin`         varchar(8) NOT NULL,
    `source_id`      text NOT NULL,
    `job_type`       text NOT NULL,
    `args`           text,
    `sched_def`      text,
    `schedule_type`  varchar(20),
    `last_run`       datetime,
    `failures`       integer,
    `state`          varchar(20) DEFAULT 'queued',
    `last_heartbeat` datetime,
    `retry_policy`   text,
    `version`        integer DEFAULT 0
);
CREATE INDEX `idx_cron_jobs_deleted_at` ON `cron_jobs` (`deleted_at`);
CREATE UNIQUE INDEX `idx_cron_jobs_uuid` ON `cron_jobs` (`uuid`);
CREATE INDEX `idx_cron_jobs_origin` ON `cron_jobs` (`origin`);
CREATE INDEX `idx_cron_jobs_source` ON `cron_jobs` (`source_id`);
CREATE INDEX `idx_cron_jobs_type` ON `cron_jobs` (`job_type`);

-- 3. Tables that depend on users table
CREATE TABLE `uploads`
(
    `id`          integer PRIMARY KEY AUTOINCREMENT,
    `created_at`  datetime,
    `updated_at`  datetime,
    `deleted_at`  datetime,
    `user_id`     integer,
    `hash`        varbinary(64),
    `cid_type`    integer,
    `mime_type`   text,
    `protocol`    text,
    `uploader_ip` text,
    `size`        integer,
    `metadata`    JSON,
    CONSTRAINT `fk_users_uploads` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE UNIQUE INDEX `idx_upload_hash_deleted_at` ON `uploads` (`hash`, `deleted_at`);
CREATE INDEX `idx_uploads_deleted_at` ON `uploads` (`deleted_at`);

CREATE TABLE `requests`
(
    `id`                   integer PRIMARY KEY AUTOINCREMENT,
    `created_at`           datetime,
    `updated_at`           datetime,
    `deleted_at`           datetime,
    `operation`            text,
    `protocol`             text,
    `status`               text,
    `status_message`       text,
    `user_id`              integer,
    `source_ip`            text,
    `hash`                 varbinary(64),
    `cid_type`             integer,
    `metadata`             JSON,
    CONSTRAINT `fk_requests_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_requests_hash` ON `requests` (`hash`);
CREATE INDEX `idx_requests_deleted_at` ON `requests` (`deleted_at`);

-- 4. Tables that depend on primary tables
CREATE TABLE `account_deletions`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `ip`         text,
    `user_id`    integer,
    CONSTRAINT `fk_account_deletions_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_account_deletions_deleted_at` ON `account_deletions` (`deleted_at`);

CREATE TABLE `cron_job_logs`
(
    `id`          integer PRIMARY KEY AUTOINCREMENT,
    `created_at`  datetime,
    `updated_at`  datetime,
    `deleted_at`  datetime,
    `cron_job_id` integer,
    `type`        text,
    `message`     text,
    CONSTRAINT `fk_cron_job_logs_cron_job` FOREIGN KEY (`cron_job_id`) REFERENCES `cron_jobs` (`id`)
);
CREATE INDEX `idx_cron_job_logs_deleted_at` ON `cron_job_logs` (`deleted_at`);

CREATE TABLE `email_verifications`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `user_id`    integer,
    `new_email`  text,
    `token`      text,
    `expires_at` datetime,
    CONSTRAINT `fk_users_email_verifications` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_email_verifications_deleted_at` ON `email_verifications` (`deleted_at`);

CREATE TABLE `password_resets`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `user_id`    integer,
    `token`      text,
    `expires_at` datetime,
    CONSTRAINT `fk_users_password_resets` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_password_resets_deleted_at` ON `password_resets` (`deleted_at`);

CREATE TABLE `pins`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `upload_id`  integer,
    `user_id`    integer,
    CONSTRAINT `fk_pins_upload` FOREIGN KEY (`upload_id`) REFERENCES `uploads` (`id`),
    CONSTRAINT `fk_pins_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
);
CREATE INDEX `idx_pins_deleted_at` ON `pins` (`deleted_at`);

CREATE TABLE `public_keys`
(
    `id`         integer PRIMARY KEY AUTOINCREMENT,
    `created_at` datetime,
    `updated_at` datetime,
    `deleted_at` datetime,
    `user_id`    integer,
    `key`        text NOT NULL,
    CONSTRAINT `fk_users_public_keys` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`),
    CONSTRAINT `uni_public_keys_key` UNIQUE (`key`)
);
CREATE INDEX `idx_public_keys_deleted_at` ON `public_keys` (`deleted_at`);

CREATE TABLE `tus_requests`
(
    `id`            integer PRIMARY KEY AUTOINCREMENT,
    `created_at`    datetime,
    `updated_at`    datetime,
    `deleted_at`    datetime,
    `request_id`    integer,
    `tus_upload_id` varchar(500),
    `upload_hash`   varbinary(64),
    `completed`     numeric,
    CONSTRAINT `fk_tus_requests_request` FOREIGN KEY (`request_id`) REFERENCES `requests` (`id`)
);
CREATE UNIQUE INDEX `idx_tus_requests_tus_upload_id` ON `tus_requests` (`tus_upload_id`);
CREATE UNIQUE INDEX `idx_tus_requests_request_id` ON `tus_requests` (`request_id`);
CREATE INDEX `idx_tus_requests_deleted_at` ON `tus_requests` (`deleted_at`);

-- +goose Down
DROP TABLE IF EXISTS `tus_requests`;
DROP TABLE IF EXISTS `public_keys`;
DROP TABLE IF EXISTS `pins`;
DROP TABLE IF EXISTS `password_resets`;
DROP TABLE IF EXISTS `email_verifications`;
DROP TABLE IF EXISTS `cron_job_logs`;
DROP TABLE IF EXISTS `account_deletions`;
DROP TABLE IF EXISTS `requests`;
DROP TABLE IF EXISTS `uploads`;
DROP INDEX IF EXISTS `idx_cron_jobs_origin`;
DROP INDEX IF EXISTS `idx_cron_jobs_source`;
DROP INDEX IF EXISTS `idx_cron_jobs_type`;
DROP TABLE IF EXISTS `cron_jobs`;
DROP TABLE IF EXISTS `users`;
DROP TABLE IF EXISTS `tus_locks`;
DROP TABLE IF EXISTS `sia_uploads`;
DROP TABLE IF EXISTS `scan_results`;
DROP TABLE IF EXISTS `s3_uploads`;
DROP TABLE IF EXISTS `hash_mappings`;
DROP TABLE IF EXISTS `access_rules`;

-- +goose Up

--
-- Table structure for table `users`
--
CREATE TABLE `users`
(
    `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`    datetime(3)  DEFAULT NULL,
    `updated_at`    datetime(3)  DEFAULT NULL,
    `deleted_at`    datetime(3)  DEFAULT NULL,
    `first_name`    longtext,
    `last_name`     longtext,
    `email`         varchar(191) DEFAULT NULL,
    `password_hash` longtext,
    `role`          longtext,
    `last_login`    datetime(3)  DEFAULT NULL,
    `last_login_ip` longtext,
    `otp_enabled`   tinyint(1)   DEFAULT '0',
    `otp_verified`  tinyint(1)   DEFAULT '0',
    `otp_secret`    longtext,
    `otp_auth_url`  longtext,
    `verified`      tinyint(1)   DEFAULT '0',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_users_email` (`email`),
    KEY `idx_users_deleted_at` (`deleted_at`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `uploads`
--
CREATE TABLE `uploads`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`  datetime(3)     DEFAULT NULL,
    `updated_at`  datetime(3)     DEFAULT NULL,
    `deleted_at`  datetime(3)     DEFAULT NULL,
    `user_id`     bigint unsigned DEFAULT NULL,
    `hash`        varbinary(64)   DEFAULT NULL,
    `cid_type`    bigint unsigned DEFAULT NULL,
    `mime_type`   longtext,
    `protocol`    longtext,
    `uploader_ip` longtext,
    `size`        bigint unsigned DEFAULT NULL,
    `metadata`    json            DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_upload_hash_deleted_at` (`hash`, `deleted_at`),
    KEY `idx_uploads_deleted_at` (`deleted_at`),
    KEY `fk_users_uploads` (`user_id`),
    CONSTRAINT `fk_users_uploads` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `cron_jobs`
--
CREATE TABLE `cron_jobs`
(
    `id`             bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`     datetime(3)     DEFAULT NULL,
    `updated_at`     datetime(3)     DEFAULT NULL,
    `deleted_at`     datetime(3)     DEFAULT NULL,
    `uuid`           binary(16)      DEFAULT NULL,
    `origin`         varchar(8)      NOT NULL,
    `source_id`      varchar(255)    NOT NULL,
    `job_type`       varchar(255)    NOT NULL,
    `args`           longtext,
    `sched_def`      longtext,
    `last_run`       datetime(3)     DEFAULT NULL,
    `failures`       bigint unsigned DEFAULT NULL,
    `state`          varchar(20)     DEFAULT 'queued',
    `last_heartbeat` datetime(3)     DEFAULT NULL,
    `version`        bigint unsigned DEFAULT '0',
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_cron_jobs_uuid` (`uuid`),
    KEY `idx_cron_jobs_deleted_at` (`deleted_at`),
    KEY `idx_cron_jobs_origin` (`origin`),
    KEY `idx_cron_jobs_source` (`source_id`),
    KEY `idx_cron_jobs_type` (`job_type`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `requests`
--
CREATE TABLE `requests`
(
    `id`                   bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`           datetime(3)     DEFAULT NULL,
    `updated_at`           datetime(3)     DEFAULT NULL,
    `deleted_at`           datetime(3)     DEFAULT NULL,
    `operation`            varchar(191)    DEFAULT NULL,
    `protocol`             longtext,
    `status`               longtext,
    `status_message`       longtext,
    `system`               tinyint(1)      DEFAULT '0',
    `user_id`              bigint unsigned DEFAULT NULL,
    `source_ip`            longtext,
    `hash_type`            bigint unsigned DEFAULT NULL,
    `hash`                 varbinary(64)   DEFAULT NULL,
    `cid_type`             bigint unsigned DEFAULT NULL,
    `upload_hash`          varbinary(64)   DEFAULT NULL,
    `upload_hash_cid_type` bigint unsigned DEFAULT NULL,
    `size`                 bigint unsigned DEFAULT NULL,
    `mime_type`            longtext,
    `metadata`             json            DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_requests_upload_hash` (`upload_hash`),
    KEY `idx_requests_deleted_at` (`deleted_at`),
    KEY `idx_request_operation_system` (`operation`, `system`),
    KEY `idx_requests_hash` (`hash`),
    KEY `fk_requests_user` (`user_id`),
    CONSTRAINT `fk_requests_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `account_deletions`
--
CREATE TABLE `account_deletions`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)     DEFAULT NULL,
    `updated_at` datetime(3)     DEFAULT NULL,
    `deleted_at` datetime(3)     DEFAULT NULL,
    `ip`         longtext,
    `user_id`    bigint unsigned DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_account_deletions_deleted_at` (`deleted_at`),
    KEY `fk_account_deletions_user` (`user_id`),
    CONSTRAINT `fk_account_deletions_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `cron_job_logs`
--
CREATE TABLE `cron_job_logs`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`  datetime(3)     DEFAULT NULL,
    `updated_at`  datetime(3)     DEFAULT NULL,
    `deleted_at`  datetime(3)     DEFAULT NULL,
    `cron_job_id` bigint unsigned DEFAULT NULL,
    `type`        longtext,
    `message`     longtext,
    PRIMARY KEY (`id`),
    KEY `idx_cron_job_logs_deleted_at` (`deleted_at`),
    KEY `fk_cron_job_logs_cron_job` (`cron_job_id`),
    CONSTRAINT `fk_cron_job_logs_cron_job` FOREIGN KEY (`cron_job_id`) REFERENCES `cron_jobs` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `email_verifications`
--
CREATE TABLE `email_verifications`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)     DEFAULT NULL,
    `updated_at` datetime(3)     DEFAULT NULL,
    `deleted_at` datetime(3)     DEFAULT NULL,
    `user_id`    bigint unsigned DEFAULT NULL,
    `new_email`  longtext,
    `token`      longtext,
    `expires_at` datetime(3)     DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_email_verifications_deleted_at` (`deleted_at`),
    KEY `fk_users_email_verifications` (`user_id`),
    CONSTRAINT `fk_users_email_verifications` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `password_resets`
--
CREATE TABLE `password_resets`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)     DEFAULT NULL,
    `updated_at` datetime(3)     DEFAULT NULL,
    `deleted_at` datetime(3)     DEFAULT NULL,
    `user_id`    bigint unsigned DEFAULT NULL,
    `token`      longtext,
    `expires_at` datetime(3)     DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_password_resets_deleted_at` (`deleted_at`),
    KEY `fk_users_password_resets` (`user_id`),
    CONSTRAINT `fk_users_password_resets` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `pins`
--
CREATE TABLE `pins`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)     DEFAULT NULL,
    `updated_at` datetime(3)     DEFAULT NULL,
    `deleted_at` datetime(3)     DEFAULT NULL,
    `upload_id`  bigint unsigned DEFAULT NULL,
    `user_id`    bigint unsigned DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_pins_deleted_at` (`deleted_at`),
    KEY `fk_pins_upload` (`upload_id`),
    KEY `fk_pins_user` (`user_id`),
    CONSTRAINT `fk_pins_upload` FOREIGN KEY (`upload_id`) REFERENCES `uploads` (`id`),
    CONSTRAINT `fk_pins_user` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `public_keys`
--
CREATE TABLE `public_keys`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)     DEFAULT NULL,
    `updated_at` datetime(3)     DEFAULT NULL,
    `deleted_at` datetime(3)     DEFAULT NULL,
    `user_id`    bigint unsigned DEFAULT NULL,
    `key`        varchar(191)    NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_public_keys_key` (`key`),
    KEY `idx_public_keys_deleted_at` (`deleted_at`),
    KEY `fk_users_public_keys` (`user_id`),
    CONSTRAINT `fk_users_public_keys` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `tus_requests`
--
CREATE TABLE `tus_requests`
(
    `id`            bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`    datetime(3)     DEFAULT NULL,
    `updated_at`    datetime(3)     DEFAULT NULL,
    `deleted_at`    datetime(3)     DEFAULT NULL,
    `request_id`    bigint unsigned DEFAULT NULL,
    `tus_upload_id` varchar(500)    DEFAULT NULL,
    `completed`     tinyint(1)      DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_tus_requests_request_id` (`request_id`),
    UNIQUE KEY `idx_tus_requests_tus_upload_id` (`tus_upload_id`),
    KEY `idx_tus_requests_deleted_at` (`deleted_at`),
    CONSTRAINT `fk_tus_requests_request` FOREIGN KEY (`request_id`) REFERENCES `requests` (`id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `access_rules`
--
CREATE TABLE `access_rules`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)  DEFAULT NULL,
    `updated_at` datetime(3)  DEFAULT NULL,
    `deleted_at` datetime(3)  DEFAULT NULL,
    `ptype`      varchar(512) DEFAULT NULL,
    `v0`         varchar(512) DEFAULT NULL,
    `v1`         varchar(512) DEFAULT NULL,
    `v2`         varchar(512) DEFAULT NULL,
    `v3`         varchar(512) DEFAULT NULL,
    `v4`         varchar(512) DEFAULT NULL,
    `v5`         varchar(512) DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_access_rule` (`ptype`(100), `v0`(100), `v1`(100), `v2`(100), `v3`(100), `v4`(100), `v5`(100)),
    KEY `idx_access_rules_deleted_at` (`deleted_at`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `hash_mappings`
--
CREATE TABLE `hash_mappings`
(
    `id`          bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`  datetime(3)   DEFAULT NULL,
    `updated_at`  datetime(3)   DEFAULT NULL,
    `deleted_at`  datetime(3)   DEFAULT NULL,
    `source_hash` varbinary(64) DEFAULT NULL,
    `target_hash` varbinary(64) DEFAULT NULL,
    `protocol`    varchar(255)  DEFAULT NULL,
    `metadata`    json          DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_hash_mapping_target` (`target_hash`),
    KEY `idx_hash_mapping_protocol` (`protocol`),
    KEY `idx_hash_mappings_deleted_at` (`deleted_at`),
    KEY `idx_hash_mapping_source` (`source_hash`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `s3_uploads`
--
CREATE TABLE `s3_uploads`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3) DEFAULT NULL,
    `updated_at` datetime(3) DEFAULT NULL,
    `deleted_at` datetime(3) DEFAULT NULL,
    `upload_id`  varchar(191)    NOT NULL,
    `bucket`     varchar(191)    NOT NULL,
    `key`        varchar(191)    NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_s3_uploads_upload_id` (`upload_id`),
    KEY `idx_s3_uploads_deleted_at` (`deleted_at`),
    KEY `idx_s3_bucket_key` (`bucket`, `key`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `scan_results`
--
CREATE TABLE `scan_results`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3)   DEFAULT NULL,
    `updated_at` datetime(3)   DEFAULT NULL,
    `deleted_at` datetime(3)   DEFAULT NULL,
    `hash`       varbinary(64) DEFAULT NULL,
    `scanner_id` varchar(191)  DEFAULT NULL,
    `passed`     tinyint(1)    DEFAULT NULL,
    `reason`     longtext,
    `metadata`   json          DEFAULT NULL,
    PRIMARY KEY (`id`),
    KEY `idx_scan_results_deleted_at` (`deleted_at`),
    KEY `idx_scan_results_hash` (`hash`),
    KEY `idx_scan_results_scanner_id` (`scanner_id`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;
--
-- Table structure for table `sia_uploads`
--
CREATE TABLE `sia_uploads`
(
    `id`         bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at` datetime(3) DEFAULT NULL,
    `updated_at` datetime(3) DEFAULT NULL,
    `deleted_at` datetime(3) DEFAULT NULL,
    `upload_id`  varchar(191)    NOT NULL,
    `bucket`     varchar(191)    NOT NULL,
    `key`        varchar(191)    NOT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uni_sia_uploads_upload_id` (`upload_id`),
    KEY `idx_sia_uploads_deleted_at` (`deleted_at`),
    KEY `idx_sia_bucket_key` (`bucket`, `key`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

--
-- Table structure for table `tus_locks`
--
CREATE TABLE `tus_locks`
(
    `id`                bigint unsigned NOT NULL AUTO_INCREMENT,
    `created_at`        datetime(3)  DEFAULT NULL,
    `updated_at`        datetime(3)  DEFAULT NULL,
    `deleted_at`        datetime(3)  DEFAULT NULL,
    `lock_id`           varchar(191) DEFAULT NULL,
    `holder_p_id`       bigint       DEFAULT NULL,
    `acquired_at`       datetime(3)  DEFAULT NULL,
    `expires_at`        datetime(3)  DEFAULT NULL,
    `release_requested` tinyint(1)   DEFAULT NULL,
    PRIMARY KEY (`id`),
    UNIQUE KEY `idx_lock_id` (`lock_id`, `deleted_at`),
    KEY `idx_tus_locks_holder_p_id` (`holder_p_id`),
    KEY `idx_tus_locks_deleted_at` (`deleted_at`)
) ENGINE = InnoDB
  DEFAULT CHARSET = utf8mb4
  COLLATE = utf8mb4_0900_ai_ci;

-- +goose Down

DROP TABLE IF EXISTS `tus_locks`;
DROP TABLE IF EXISTS `sia_uploads`;
DROP TABLE IF EXISTS `schema_migrations`;
DROP TABLE IF EXISTS `scan_results`;
DROP TABLE IF EXISTS `s3_uploads`;
DROP TABLE IF EXISTS `hash_mappings`;
DROP TABLE IF EXISTS `access_rules`;
DROP TABLE IF EXISTS `tus_requests`;
DROP TABLE IF EXISTS `public_keys`;
DROP TABLE IF EXISTS `pins`;
DROP TABLE IF EXISTS `password_resets`;
DROP TABLE IF EXISTS `email_verifications`;
DROP TABLE IF EXISTS `cron_job_logs`;
DROP TABLE IF EXISTS `account_deletions`;
DROP TABLE IF EXISTS `requests`;
DROP TABLE IF EXISTS `cron_jobs`;
DROP TABLE IF EXISTS `uploads`;
DROP TABLE IF EXISTS `users`;

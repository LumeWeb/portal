-- +goose Up

ALTER TABLE `oauth_clients`
    ADD COLUMN `client_uri` longtext AFTER `client_id`;

-- +goose Down

ALTER TABLE `oauth_clients`
    DROP COLUMN `client_uri`;

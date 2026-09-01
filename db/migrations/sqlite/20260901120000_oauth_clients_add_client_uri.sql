-- +goose Up

ALTER TABLE `oauth_clients`
    ADD COLUMN `client_uri` text;

-- +goose Down

ALTER TABLE `oauth_clients`
    DROP COLUMN `client_uri`;

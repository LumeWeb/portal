package config

import "errors"

var (
	ErrInvalidServiceConfig = errors.New("service config must be of type config.ServiceConfig")
)

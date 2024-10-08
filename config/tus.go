package config

import (
	"errors"
	"github.com/samber/lo"
)

type TusConfig struct {
	LockerMode string `config:"locker_mode"`
}

func (t TusConfig) Validate() error {
	if t.LockerMode != "" && !lo.Contains([]string{"db", "redis"}, t.LockerMode) {
		return errors.New("tus_locker_mode must be one of: db, redis")
	}

	return nil
}
func (c Config) Defaults() map[string]any {
	defaults := map[string]any{
		"tus_locker_mode": "db",
	}

	return defaults
}

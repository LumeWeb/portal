// Package fsadapter provides a file system adapter for Casbin.
// This code is based on https://github.com/naucon/casbin-fs-adapter
// Copyright (c) 2015 naucon (MIT License)
// Modified for Casbin v3 compatibility
package fsadapter

import (
	"io/fs"

	"github.com/casbin/casbin/v3/model"
)

func NewModel(fsys fs.FS, filePath string) (model.Model, error) {
	b, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return nil, err
	}

	return model.NewModelFromString(string(b))
}

SHELL := /bin/bash

.DEFAULT_GOAL := default

.PHONY: default build go-build

default: build

build: go-build
	@echo "Build completed."

go-build:
ifeq ($(ENV),dev)
	go mod vendor
	go build -gcflags="all=-N -l" -o portal ./cmd/portal
else
	go build -ldflags='-s -w -linkmode external -extldflags "-static"' -o portal ./cmd/portal
endif
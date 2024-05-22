BUILD_TAGS := s5
SHELL := /bin/bash

.DEFAULT_GOAL := default

.PHONY: default build go-build generate-api-swagger build-sync generate-proto build-node build-dashboard

default: build

build: go-build
	@echo "Build completed."

go-build: go-mod-download generate-api-swagger build-dashboard build-sync-node generate-proto
	go build -tags "$(BUILD_TAGS)" -gcflags="all=-N -l" -o portal ./cmd/portal

go-mod-download:
	go mod download

go-get-xz:
	go get github.com/ulikunitz/xz

generate-api-swagger: go-mod-download go-get-xz
	go generate api/swagger/swagger.go

build-sync: generate-proto build-node

generate-proto:
	cd ./sync/proto && buf generate

build-node:
	cd ./sync/node && bash build.sh

build-dashboard:
	cd ./api/account/app && \
	pnpm install && \
	rm -rf .nx && \
	pnpm build:portal-dashboard

build-sync-node:
	cd ./sync/node && pnpm install && bash build.sh
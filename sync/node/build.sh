#!/usr/bin/env bash
mkdir -p app/proto
cp -r ./node_modules/@grpc/reflection/build/proto/grpc ./app/proto
cp -r ./node_modules/grpc-health-check/proto/health ./app/proto
mkdir -p app/app/app/build/Release && cp ./node_modules/sodium-native/prebuilds/linux-x64/sodium-native.node app/app/app/build/Release
mkdir -p src/generated
./node_modules/protobufjs-cli/bin/pbjs -t json ../proto/protocol.proto > src/generated/protobuf.json
./node_modules/protobufjs-cli/bin/pbjs -t json ../../vendor/github.com/hashicorp/go-plugin/internal/plugin/grpc_stdio.proto > src/generated/grpc_stdio.json
./node_modules/.bin/rollup -c rollup.config.js --silent

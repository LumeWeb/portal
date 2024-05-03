#!/usr/bin/env bash
mkdir -p app/proto
cp -r ./node_modules/@grpc/reflection/build/proto/grpc ./app/proto
cp -r ./node_modules/grpc-health-check/proto/health ./app/proto
mkdir -p app/app/app/build/Release && cp ./node_modules/sodium-native/prebuilds/linux-x64/sodium-native.node app/app/app/build/Release
mkdir -p src/generated
./node_modules/protobufjs-cli/bin/pbjs -t json ../proto/protocol.proto > src/generated/protobuf.json


# Check if vendor exists
if [ -d "../../vendor/github.com/hashicorp" ]; then
    proto_file="$(readlink -f ../../vendor)/github.com/hashicorp/go-plugin/internal/plugin/grpc_stdio.proto"
# Check if GOPATH is set
elif [ -d "$GOPATH/go/pkg/mod" ]; then
    proto_file=$(find "$GOPATH/go/pkg/mod/github.com/hashicorp" -name grpc_stdio.proto -print -quit)
# Check if $HOME/go/pkg/mod exists
elif [ -d "$HOME/go/pkg/mod/github.com/hashicorp" ]; then
    proto_file="$(find "$HOME/go/pkg/mod/github.com/hashicorp" -name grpc_stdio.proto -print -quit)"
else
    echo "Error: Could not find the parent directory of github.com/hashicorp"
    exit 1
fi

# Run protobufjs-cli command
./node_modules/protobufjs-cli/bin/pbjs -t json "$proto_file" > src/generated/grpc_stdio.json
./node_modules/.bin/rollup -c rollup.config.js --silent

#!/usr/bin/env bash

export NVM_DIR="$HOME/.nvm"


download() {
  curl -s -S -L https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer | bash
  curl -s -S -L https://raw.githubusercontent.com/nvm-sh/nvm/v0.39.3/install.sh | bash
}

install() {
  source "$HOME/.gvm/scripts/gvm"
  gvm install go${GO_VERSION} -B
  gvm use go${GO_VERSION}

  source "$NVM_DIR/nvm.sh"
  nvm install ${NODE_VERSION}
  nvm use ${NODE_VERSION}

  npm install -g pnpm@8.8.0

  GO111MODULE=on GOBIN=/usr/local/bin go install github.com/bufbuild/buf/cmd/buf@v1.32.0
}

set_env() {
  source "$HOME/.gvm/scripts/gvm"
  gvm use go${GO_VERSION}
  source "$NVM_DIR/nvm.sh"
  nvm use ${NODE_VERSION}
}
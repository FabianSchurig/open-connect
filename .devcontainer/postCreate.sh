#!/usr/bin/env bash
# .devcontainer/postCreate.sh — install pinned tools used by the project.
# Versions are pinned (Epic A) so a fresh clone is reproducible.
set -euo pipefail

BUF_VERSION="1.59.0"
GOLANG_MIGRATE_VERSION="v4.18.4"
NATS_CLI_VERSION="0.3.0"
PROTOC_VERSION="34.1"

sudo apt-get update -qq
sudo apt-get install -y --no-install-recommends \
    bmap-tools \
    qemu-system-x86 qemu-utils \
    postgresql-client \
    jq curl unzip ca-certificates

# protoc
PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip"
curl -fsSLO "https://github.com/protocolbuffers/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}"
sudo unzip -q -o "${PROTOC_ZIP}" -d /usr/local
sudo chmod -R +x /usr/local/bin
rm "${PROTOC_ZIP}"

# buf
sudo curl -fsSL "https://github.com/bufbuild/buf/releases/download/v${BUF_VERSION}/buf-Linux-x86_64" -o /usr/local/bin/buf
sudo chmod +x /usr/local/bin/buf

# golang-migrate
GO_MIGRATE_TGZ="migrate.linux-amd64.tar.gz"
curl -fsSL "https://github.com/golang-migrate/migrate/releases/download/${GOLANG_MIGRATE_VERSION}/${GO_MIGRATE_TGZ}" -o /tmp/${GO_MIGRATE_TGZ}
sudo tar -C /usr/local/bin -xzf /tmp/${GO_MIGRATE_TGZ} migrate
rm /tmp/${GO_MIGRATE_TGZ}

# NATS CLI
NATS_TGZ="nats-${NATS_CLI_VERSION}-linux-amd64.zip"
curl -fsSLO "https://github.com/nats-io/natscli/releases/download/v${NATS_CLI_VERSION}/${NATS_TGZ}" || true
if [[ -f "${NATS_TGZ}" ]]; then
    unzip -q -o "${NATS_TGZ}"
    sudo mv "nats-${NATS_CLI_VERSION}-linux-amd64/nats" /usr/local/bin/
    rm -rf "${NATS_TGZ}" "nats-${NATS_CLI_VERSION}-linux-amd64"
fi

# protoc-gen-go for codegen
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest

echo "open-connect devcontainer ready:"
echo "  go=$(go version)"
echo "  cargo=$(cargo --version)"
echo "  buf=$(buf --version)"
echo "  protoc=$(protoc --version)"
echo "  migrate=$(migrate -version 2>&1 || true)"
echo "  bmaptool=$(bmaptool --version 2>&1 | head -1)"

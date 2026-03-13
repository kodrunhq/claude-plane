#!/usr/bin/env bash
set -euo pipefail

VERSION="${VERSION:-dev}"
COMMIT="${COMMIT:-unknown}"
DATE="${DATE:-unknown}"
LDFLAGS="-X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Version=${VERSION} -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Commit=${COMMIT} -X github.com/kodrunhq/claude-plane/internal/shared/buildinfo.Date=${DATE}"
OUT_DIR="internal/server/agentdl/binaries"

mkdir -p "${OUT_DIR}"

PLATFORMS=("linux/amd64" "linux/arm64" "darwin/amd64" "darwin/arm64")

for platform in "${PLATFORMS[@]}"; do
    IFS='/' read -r os arch <<< "${platform}"
    output="${OUT_DIR}/claude-plane-agent-${os}-${arch}"
    echo "Building agent for ${os}/${arch}..."
    GOOS="${os}" GOARCH="${arch}" go build -ldflags "${LDFLAGS}" -o "${output}" ./cmd/agent
done

echo "Agent binaries built in ${OUT_DIR}"

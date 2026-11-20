#!/usr/bin/env bash
set -euo pipefail

# Cross-compile release archives into dist/.
# Usage: scripts/build-release.sh [version]   (default: git describe)
#
# Windows is not a target: internal/process uses POSIX process groups.

PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$PROJECT_DIR"

VERSION="${1:-$(git describe --tags --always --dirty)}"
COMMIT="$(git rev-parse HEAD)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
TARGETS=(linux/amd64 linux/arm64 darwin/amd64 darwin/arm64)

rm -rf dist
mkdir -p dist

for target in "${TARGETS[@]}"; do
  os="${target%/*}"
  arch="${target#*/}"
  name="wrkmn_${VERSION#v}_${os}_${arch}"
  echo "building $name"
  mkdir -p "dist/$name"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath \
    -ldflags "-s -w -X wrkmn/cmd.Version=$VERSION -X wrkmn/cmd.Commit=$COMMIT -X wrkmn/cmd.BuildDate=$BUILD_DATE" \
    -o "dist/$name/wrkmn" .
  cp README.md "dist/$name/"
  tar -C dist -czf "dist/$name.tar.gz" "$name"
  rm -rf "dist/$name"
done

(cd dist && sha256sum ./*.tar.gz | sed "s# \./# #" > checksums.txt)
ls -lh dist

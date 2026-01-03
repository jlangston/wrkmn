#!/usr/bin/env bash
set -euo pipefail

# Build and install wrkmn to ~/bin

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"
BIN_DIR="${HOME}/bin"
BINARY_NAME="wrkmn"

# Get version info
VERSION="${VERSION:-$(git -C "$PROJECT_DIR" describe --tags 2>/dev/null || echo "dev")}"
COMMIT="${COMMIT:-$(git -C "$PROJECT_DIR" rev-parse --short HEAD 2>/dev/null || echo "none")}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

echo "Building wrkmn..."
echo "  Version: $VERSION"
echo "  Commit:  $COMMIT"
echo "  Date:    $BUILD_DATE"

# Build with version info
cd "$PROJECT_DIR"
go build -ldflags "-X wrkmn/cmd.Version=$VERSION -X wrkmn/cmd.Commit=$COMMIT -X wrkmn/cmd.BuildDate=$BUILD_DATE" -o "$BINARY_NAME" .

# Create ~/bin if it doesn't exist
mkdir -p "$BIN_DIR"

# Remove old symlink or binary if exists
if [[ -L "$BIN_DIR/$BINARY_NAME" ]] || [[ -f "$BIN_DIR/$BINARY_NAME" ]]; then
    rm "$BIN_DIR/$BINARY_NAME"
fi

# Create symlink
ln -s "$PROJECT_DIR/$BINARY_NAME" "$BIN_DIR/$BINARY_NAME"

echo ""
echo "Installed: $BIN_DIR/$BINARY_NAME -> $PROJECT_DIR/$BINARY_NAME"
echo ""
echo "Make sure ~/bin is in your PATH:"
echo "  export PATH=\"\$HOME/bin:\$PATH\""

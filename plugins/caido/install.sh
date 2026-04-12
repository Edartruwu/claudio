#!/usr/bin/env bash
set -e

REPO="Abraxas-365/claudio-plugin-caido"
INSTALL_DIR="$HOME/.claudio/plugins"
BINARY="$INSTALL_DIR/caido"

# Detect OS + arch
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported arch: $ARCH"; exit 1 ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

ASSET="caido-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${ASSET}"

# Download
mkdir -p "$INSTALL_DIR"
if command -v curl &>/dev/null; then
  curl -fsSL "$URL" -o "$BINARY" || { echo "Download failed. Build from source: cd plugins/caido && make install"; exit 1; }
elif command -v wget &>/dev/null; then
  wget -q "$URL" -O "$BINARY" || { echo "Download failed. Build from source: cd plugins/caido && make install"; exit 1; }
else
  echo "Need curl or wget. Build from source: cd plugins/caido && go build -o $BINARY ."; exit 1
fi

chmod +x "$BINARY"
echo "Binary installed to $BINARY"
echo ""
"$BINARY" setup
echo ""
echo "Done. Restart Claudio to activate plugin."

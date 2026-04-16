#!/bin/sh
set -e

REPO="momemoV01/udit"

# Optional flags: --no-completion (also via UDIT_NO_COMPLETION=1),
#                 --no-checksum  (also via UDIT_NO_CHECKSUM=1).
NO_COMPLETION="${UDIT_NO_COMPLETION:-0}"
NO_CHECKSUM="${UDIT_NO_CHECKSUM:-0}"
for arg in "$@"; do
  case "$arg" in
    --no-completion) NO_COMPLETION=1 ;;
    --no-checksum)   NO_CHECKSUM=1 ;;
  esac
done

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux)  ;;
  darwin) ;;
  *)      echo "Unsupported OS: $OS (use Windows instructions in README)"; exit 1 ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  aarch64|arm64)  ARCH="arm64" ;;
  *)              echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

INSTALL_DIR="$HOME/.local/bin"
mkdir -p "$INSTALL_DIR"

BINARY="udit-${OS}-${ARCH}"
URL="https://github.com/${REPO}/releases/latest/download/${BINARY}"

echo "Downloading udit for ${OS}/${ARCH}..."
curl -fsSL "$URL" -o "$INSTALL_DIR/udit"

# Checksum verification (skip with --no-checksum or UDIT_NO_CHECKSUM=1).
if [ "$NO_CHECKSUM" != "1" ]; then
  SUMS_URL="https://github.com/${REPO}/releases/latest/download/SHA256SUMS.txt"
  SUMS_FILE="$(mktemp)"
  if curl -fsSL "$SUMS_URL" -o "$SUMS_FILE" 2>/dev/null; then
    EXPECTED=$(grep "${BINARY}$" "$SUMS_FILE" | awk '{print $1}')
    ACTUAL=$(sha256sum "$INSTALL_DIR/udit" | awk '{print $1}')
    rm -f "$SUMS_FILE"
    if [ -z "$EXPECTED" ]; then
      echo "Warning: no checksum entry for ${BINARY} (skipping verification)."
    elif [ "$EXPECTED" != "$ACTUAL" ]; then
      echo "Checksum mismatch! Expected: $EXPECTED, Got: $ACTUAL"
      rm -f "$INSTALL_DIR/udit"
      exit 1
    else
      echo "Checksum verified."
    fi
  else
    rm -f "$SUMS_FILE"
    echo "Warning: could not download checksums (skipping verification)."
  fi
fi

chmod +x "$INSTALL_DIR/udit"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    export PATH="$INSTALL_DIR:$PATH"
    LINE="export PATH=\"$INSTALL_DIR:\$PATH\""
    SHELL_NAME="$(basename "$SHELL")"
    case "$SHELL_NAME" in
      zsh)  RC_FILE="$HOME/.zshrc" ;;
      bash) RC_FILE="$HOME/.bashrc" ;;
      *)    RC_FILE="$HOME/.profile" ;;
    esac
    touch "$RC_FILE"
    echo "$LINE" >> "$RC_FILE"
    echo "Added $INSTALL_DIR to PATH (restart shell to apply)" ;;
esac

echo "Installed udit to $INSTALL_DIR/udit"
"$INSTALL_DIR/udit" version

# Auto-install shell completion. Best-effort — if it fails (unsupported
# shell, locked-down rc file, etc.) the install itself still succeeds.
# Skip with --no-completion or UDIT_NO_COMPLETION=1.
if [ "$NO_COMPLETION" != "1" ]; then
  "$INSTALL_DIR/udit" completion install || \
    echo "Note: shell completion install skipped (run \`udit completion install --shell <bash|zsh|fish>\` later)."
fi

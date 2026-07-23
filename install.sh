#!/bin/sh
set -e

REPO="plinthlol/alloyctl"
BINARY="alloyctl"

case "$(uname -s)" in
    Linux*)     OS="linux";;
    Darwin*)    OS="darwin";;
    CYGWIN*|MINGW*|MSYS*)  OS="windows";;
    *)
        echo "Error: Unsupported OS: $(uname -s)"
        exit 1
        ;;
esac

case "$(uname -m)" in
    x86_64|amd64)   ARCH="amd64";;
    aarch64|arm64)   ARCH="arm64";;
    *)
        echo "Error: Unsupported architecture: $(uname -m)"
        exit 1
        ;;
esac

if [ "$OS" = "windows" ]; then
    FILENAME="${BINARY}-${OS}-${ARCH}.exe"
else
    FILENAME="${BINARY}-${OS}-${ARCH}"
fi

URL="https://github.com/${REPO}/releases/latest/download/${FILENAME}"

echo "Downloading ${FILENAME}..."
curl -sL -o "${BINARY}" "${URL}"
chmod +x "${BINARY}"

echo "Installed ${BINARY} to $(pwd)/${BINARY}"
echo ""
echo "Move it to your PATH:"
echo "  sudo mv ${BINARY} /usr/local/bin/"
echo ""
echo "Or add current directory to PATH:"
echo "  export PATH=\"\$(pwd):\${PATH}\""

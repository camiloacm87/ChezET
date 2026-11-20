#!/bin/bash
# Lanzador de Ajedrez en Red para macOS (elige el binario según el procesador).
cd "$(dirname "$0")" || exit 1
xattr -dr com.apple.quarantine . 2>/dev/null
chmod +x ./ajedrez-mac-* 2>/dev/null
if [ "$(uname -m)" = "arm64" ]; then
  exec ./ajedrez-mac-applesilicon "$@"
else
  exec ./ajedrez-mac-intel "$@"
fi

#!/bin/bash
# Compila la app de macOS con ventana propia: "dist/Ajedrez en Red.app"
# (universal: Apple Silicon + Intel) y la empaqueta en dist/AjedrezEnRed-Mac.zip.
#
# Debe ejecutarse EN un Mac con:
#   - Go 1.22 o superior        (https://go.dev/dl/)
#   - Xcode Command Line Tools  (xcode-select --install)
# Uso:  bash build_mac.sh
set -euo pipefail
cd "$(dirname "$0")"

APP="dist/Ajedrez en Red.app"
rm -rf "$APP" dist/AjedrezEnRed-Mac.zip
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"

export CGO_ENABLED=1
export MACOSX_DEPLOYMENT_TARGET=12.0
echo "Compilando para Apple Silicon…"
GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o dist/.ajedrez-arm64 .
echo "Compilando para Intel…"
GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o dist/.ajedrez-amd64 .
lipo -create -output "$APP/Contents/MacOS/AjedrezEnRed" dist/.ajedrez-arm64 dist/.ajedrez-amd64
rm -f dist/.ajedrez-arm64 dist/.ajedrez-amd64

cp packaging/mac/Info.plist "$APP/Contents/Info.plist"

echo "Generando icono…"
TMP=$(mktemp -d)
ICONSET="$TMP/AppIcon.iconset"
mkdir -p "$ICONSET"
for s in 16 32 128 256 512; do
  sips -z $s $s packaging/icon/icon.png --out "$ICONSET/icon_${s}x${s}.png" >/dev/null
  sips -z $((s*2)) $((s*2)) packaging/icon/icon.png --out "$ICONSET/icon_${s}x${s}@2x.png" >/dev/null
done
iconutil -c icns "$ICONSET" -o "$APP/Contents/Resources/AppIcon.icns"
rm -rf "$TMP"

echo "Firmando (ad-hoc)…"
codesign --force --deep --sign - "$APP"

(cd dist && ditto -c -k --keepParent "Ajedrez en Red.app" "AjedrezEnRed-Mac.zip")
echo
echo "Listo: $APP"
echo "       dist/AjedrezEnRed-Mac.zip (para repartir)"

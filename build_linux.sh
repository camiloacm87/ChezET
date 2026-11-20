#!/bin/bash
# Compila Ajedrez en Red para Linux (x86_64) y crea
#   Ejecutables/Linux/AjedrezEnRed-Linux.tar.gz
# con la versión de ventana propia (WebKitGTK), una de respaldo que usa el
# navegador, el icono y un instalador.
#
# Debe ejecutarse EN Linux con Go, gcc/g++, pkg-config y las librerías de desarrollo:
#   Ubuntu/Debian: sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
# Uso:  bash build_linux.sh
set -euo pipefail
cd "$(dirname "$0")"

OUT="Ejecutables/Linux"
TMP=$(mktemp -d)
PKG="$TMP/AjedrezEnRed-Linux"
mkdir -p "$PKG" "$OUT"

# webview_go pide webkit2gtk-4.0; las distribuciones actuales traen la 4.1 (misma API).
if ! pkg-config --exists webkit2gtk-4.0 && pkg-config --exists webkit2gtk-4.1; then
  mkdir -p "$TMP/pc"
  printf 'Name: webkit2gtk-4.0\nDescription: alias de webkit2gtk-4.1\nVersion: 2.0\nRequires: webkit2gtk-4.1\n' \
    > "$TMP/pc/webkit2gtk-4.0.pc"
  export PKG_CONFIG_PATH="$TMP/pc${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}"
fi

echo "Compilando versión con ventana propia…"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$PKG/AjedrezEnRed" .
echo "Compilando versión de respaldo (navegador)…"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o "$PKG/AjedrezEnRed-navegador" .

cp packaging/icon/icon_256.png "$PKG/icono.png"
cp packaging/linux/instalar.sh packaging/linux/LEEME.txt "$PKG/"
chmod +x "$PKG/AjedrezEnRed" "$PKG/AjedrezEnRed-navegador" "$PKG/instalar.sh"

rm -f "$OUT/AjedrezEnRed-Linux.tar.gz"
tar -czf "$OUT/AjedrezEnRed-Linux.tar.gz" -C "$TMP" AjedrezEnRed-Linux
rm -rf "$TMP"
echo "Listo: $OUT/AjedrezEnRed-Linux.tar.gz"

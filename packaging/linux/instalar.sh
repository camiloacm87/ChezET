#!/bin/bash
# Instala Ajedrez en Red para el usuario actual (sin sudo) y crea el acceso
# en el menú de aplicaciones.
set -e
cd "$(dirname "$0")"

DEST="$HOME/.local/share/ajedrez-en-red"
APPS="$HOME/.local/share/applications"
ICONS="$HOME/.local/share/icons/hicolor/256x256/apps"
mkdir -p "$DEST" "$APPS" "$ICONS"

cp AjedrezEnRed AjedrezEnRed-navegador "$DEST/"
chmod +x "$DEST/AjedrezEnRed" "$DEST/AjedrezEnRed-navegador"
cp icono.png "$ICONS/ajedrez-en-red.png"

EXE="$DEST/AjedrezEnRed"
TERM_FLAG=false
if ldd "$EXE" 2>/dev/null | grep -q "not found"; then
  echo "Aviso: falta WebKitGTK; se usará la versión que abre el navegador."
  echo "  Para tener ventana propia instala:"
  echo "    Ubuntu/Debian: sudo apt install libwebkit2gtk-4.1-0"
  echo "    Fedora:        sudo dnf install webkit2gtk4.1"
  echo "  y vuelve a ejecutar ./instalar.sh"
  EXE="$DEST/AjedrezEnRed-navegador"
  TERM_FLAG=true   # se muestra una terminal; al cerrarla se cierra el juego
fi

cat > "$APPS/ajedrez-en-red.desktop" <<DESKTOP
[Desktop Entry]
Type=Application
Name=Ajedrez en Red
Comment=Ajedrez para dos jugadores por la red local
Exec="$EXE"
Icon=ajedrez-en-red
Terminal=$TERM_FLAG
Categories=Game;BoardGame;
DESKTOP
chmod +x "$APPS/ajedrez-en-red.desktop"
command -v update-desktop-database >/dev/null && update-desktop-database "$APPS" >/dev/null 2>&1 || true

echo "Listo: busca \"Ajedrez en Red\" en el menú de aplicaciones."

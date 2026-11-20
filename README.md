# ♞ Ajedrez en Red

Ajedrez para dos jugadores por la red local del trabajo. Sin instalación, sin internet y sin servidores externos.

## Descargar

👉 **[Última versión](https://github.com/camiloacm87/ChezET/releases/latest)**: `AjedrezEnRed.exe` (Windows) y `AjedrezEnRed-Mac.zip` (Mac).

## Apps

| Sistema | Archivo | Cómo se obtiene |
|---|---|---|
| **Windows** | `Ejecutables/Windows/AjedrezEnRed.exe` — app con ventana propia e icono | `.\build.ps1` en Windows |
| **macOS** | `Ejecutables/Mac/Ajedrez en Red.app` (en `AjedrezEnRed-Mac.zip`) — app con ventana propia, universal Intel + Apple Silicon | `bash build_mac.sh` **en un Mac**, o automáticamente con GitHub Actions |
| macOS (provisional) | `Ejecutables/Mac/AjedrezEnRed-Mac-sin-ventana.zip` — funciona en el navegador | `.\build.ps1` en Windows |
| Android | (próximamente) | ver [ANDROID.md](ANDROID.md) |

**Windows:** doble clic en `AjedrezEnRed.exe`. Si sale SmartScreen: *Más información → Ejecutar de todas formas*.
Necesita Microsoft Edge WebView2, que ya viene en Windows 10/11 (si faltara, el juego se abre en el navegador).

**Mac:** descomprimir y arrastrar *Ajedrez en Red* a Aplicaciones. La primera vez: clic derecho → *Abrir*
(en macOS 15+: *Ajustes del Sistema → Privacidad y seguridad → Abrir igualmente*). Requiere macOS 12+.

> Apple no permite crear apps de Mac con ventana desde Windows: la compilación tiene que hacerse en un Mac.
> Si no tienes uno, sube el proyecto a GitHub: el flujo `.github/workflows/build.yml` compila la app de Mac
> (y la de Windows) en los servidores de GitHub y deja los archivos para descargar en la pestaña *Actions*.

## Cómo jugar

1. Los dos jugadores abren la app.
2. Uno pulsa **Crear partida** (elige blancas, negras o al azar).
3. El otro pulsa **Unirse** sobre la partida que aparece en *Partidas encontradas en la red*,
   o escribe la dirección que se muestra en el equipo del anfitrión (p. ej. `192.168.1.25:8765`).
4. Mover: clic en la pieza y clic en el destino, o arrastrar y soltar.

Incluye todas las reglas (enroque, captura al paso, coronación, jaque mate, rey ahogado, tablas por
material insuficiente, 50 movimientos y triple repetición), ofrecer tablas, rendirse, revancha
(con cambio de colores), chat y reconexión.

**Firewall:** la primera vez Windows/macOS preguntarán si permitir conexiones de red: pulsar *Permitir*.
Puertos: TCP 8765 (partida) y UDP 8766 (descubrimiento). Si la red bloquea el descubrimiento,
unirse escribiendo la IP sigue funcionando.

Opciones: `-port 9000` (otro puerto), `-browser` (usar el navegador en lugar de la ventana propia).

## Compilar

Requiere [Go](https://go.dev/dl/) 1.22+.

```powershell
.\build.ps1
```

## Estructura

- `main.go`, `window_*.go` — app de escritorio: ventana propia (WebView2 en Windows, WebKit en Mac).
- `server/server.go` — núcleo: servidor local, sincronización en tiempo real (SSE), descubrimiento UDP.
- `server/web/` — interfaz y motor de reglas (`chess.js`, validado con pruebas perft).
- `packaging/` — icono, `Info.plist` y lanzador de Mac.
- `tools/mkrsrc` — incrusta icono y manifiesto en el `.exe`; `tools/mkzip` — zip con permisos Unix.
- `build.ps1`, `build_mac.sh`, `.github/workflows/build.yml` — compilación.

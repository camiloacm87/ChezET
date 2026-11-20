# ♞ Ajedrez en Red

Ajedrez para dos jugadores por la red local del trabajo. Sin instalación, sin internet y sin servidores externos.

## Descargar

Los ejecutables están en la carpeta **[Ejecutables](Ejecutables)** de este repositorio
y también en **[Releases](https://github.com/camiloacm87/ChezET/releases/latest)**:

| Sistema | Archivo |
|---|---|
| **Windows 10/11** | [`Ejecutables/Windows/AjedrezEnRed.exe`](Ejecutables/Windows/AjedrezEnRed.exe) — app con ventana propia e icono |
| **macOS 12+** (Intel y Apple Silicon) | [`Ejecutables/Mac/AjedrezEnRed-Mac.zip`](Ejecutables/Mac/AjedrezEnRed-Mac.zip) — contiene *Ajedrez en Red.app* |
| Android | (próximamente) — ver [ANDROID.md](ANDROID.md) |

Para descargar un archivo desde la carpeta: ábrelo y pulsa el botón de descarga (⤓) de la derecha.

**Windows:** doble clic en `AjedrezEnRed.exe`. Si sale SmartScreen: *Más información → Ejecutar de todas formas*.
Necesita Microsoft Edge WebView2, que ya viene en Windows 10/11 (si faltara, el juego se abre en el navegador).

**Mac:** descomprimir y arrastrar *Ajedrez en Red* a Aplicaciones. La primera vez: clic derecho → *Abrir*
(en macOS 15+: *Ajustes del Sistema → Privacidad y seguridad → Abrir igualmente*). Requiere macOS 12+.

> Apple no permite crear apps de Mac con ventana desde Windows: la compilación tiene que hacerse en un Mac.
> Si no tienes uno, sube el proyecto a GitHub: el flujo `.github/workflows/build.yml` compila la app de Mac
> (y la de Windows) en los servidores de GitHub y deja los archivos para descargar en la pestaña *Actions*
> (y en *Releases* al subir una etiqueta `v*`).

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
- `packaging/` — icono e `Info.plist` de Mac.
- `tools/mkrsrc` — incrusta icono y manifiesto en el `.exe`.
- `Ejecutables/` — apps compiladas listas para descargar.
- `build.ps1`, `build_mac.sh`, `.github/workflows/build.yml` — compilación.

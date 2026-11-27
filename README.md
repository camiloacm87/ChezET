# ♞ Juegos en Red

Ajedrez, Damas, UNO y Golpeado para jugar por la red local. Sin instalación, sin internet y sin servidores externos.

## Descargar

Los ejecutables están en la carpeta **[Ejecutables](Ejecutables)** de este repositorio
y también en **[Releases](https://github.com/camiloacm87/ChezET/releases/latest)**:

| Sistema | Archivo |
|---|---|
| **Windows 10/11** | [`Ejecutables/Windows/AjedrezEnRed.exe`](Ejecutables/Windows/AjedrezEnRed.exe) — app con ventana propia e icono |
| **macOS 12+** (Intel y Apple Silicon) | [`Ejecutables/Mac/AjedrezEnRed-Mac.zip`](Ejecutables/Mac/AjedrezEnRed-Mac.zip) — contiene *Ajedrez en Red.app* |
| **Linux** (64 bits) | [`Ejecutables/Linux/AjedrezEnRed-Linux.tar.gz`](Ejecutables/Linux/AjedrezEnRed-Linux.tar.gz) — descomprimir y ejecutar `./instalar.sh` |
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

1. Los jugadores abren la app.
2. Uno elige **Ajedrez**, **Damas**, **UNO** o **Golpeado** y pulsa **Crear partida**. En Ajedrez puede elegir color; en UNO y Golpeado se crea una sala (hasta 8 jugadores) y el anfitrión pulsa **Iniciar partida** cuando haya 2 o más.
3. Los demás pulsan **Unirse** sobre la partida que aparece en *Partidas encontradas en la red*,
   o escribe la dirección que se muestra en el equipo del anfitrión (p. ej. `192.168.1.25:8765`).
4. En Ajedrez se puede elegir sin límite, un tiempo máximo por turno o un reloj total por jugador.
   En Ajedrez y Damas se mueve haciendo clic en una pieza y luego en el destino.

**Juegos:** Ajedrez incluye todas las reglas (enroque, captura al paso, coronación, jaque mate, rey ahogado, tablas por
material insuficiente, 50 movimientos y triple repetición), ofrecer tablas, rendirse, revancha
(con cambio de colores), chat y reconexión. La lista muestra las casillas de salida y llegada de cada jugada.
Quienes entren a una partida ya completa como espectadores también pueden participar en el chat.
Damas es para dos y obliga a capturar cuando hay una captura disponible. UNO usa cartas clásicas, con comodines,
reversa, saltos y cartas +2/+4; no permite acumular penalizaciones. Golpeado permite de 2 a 8 jugadores (se usan
dos barajas cuando juegan 8), reparte 7 cartas (8 al primer jugador, que descarta para iniciar), y permite formar
tercias/escaleras y enchufar cartas.
Tras una vuelta completa se puede golpear al inicio del turno; si quien golpea no tiene estrictamente menos puntos
que todos, se duplica su puntaje. Cada golpeo cierra la partida y muestra los puntajes de esa mano.

**Red y desconexión:**
- Al abrir la app, el equipo **no es accesible desde la red**: la interfaz solo escucha dentro del propio equipo.
  El puerto de red se abre únicamente al **crear** una partida y se cierra al terminarla, al salir o al cerrar la app.
- Si un jugador cierra la app o pulsa *Salir de la partida*, al otro le aparece el aviso, la partida termina
  y vuelve a la pantalla de inicio. Si se corta la red, el aviso llega a los 10 segundos.
- La app **empieza siempre desde cero**: no guarda partidas, nombres ni direcciones entre usos.

**Firewall:** la primera vez que crees una partida, Windows/macOS preguntarán si permitir conexiones de red: pulsar *Permitir*.
Puertos: TCP 8765 (partida) y UDP 8766 (descubrimiento). Si la red bloquea el descubrimiento,
unirse escribiendo la IP sigue funcionando.

Opciones: `-port 9000` (otro puerto), `-browser` (usar el navegador en lugar de la ventana propia).

## Compilar

Requiere [Go](https://go.dev/dl/) 1.22+.

```powershell
.\build.ps1
```

## Estructura

- `main.go`, `window_*.go` — app de escritorio: ventana propia (WebView2 en Windows, WebKit en Mac, WebKitGTK en Linux).
- `server/server.go` — núcleo: servidor local, sincronización en tiempo real (SSE), descubrimiento UDP.
- `server/games.go` — reglas de Damas, UNO y Golpeado, validadas por el servidor.
- `server/web/` — interfaz y motor de reglas (`chess.js`, validado con pruebas perft).
- `packaging/` — icono e `Info.plist` de Mac.
- `tools/mkrsrc` — incrusta icono y manifiesto en el `.exe`.
- `Ejecutables/` — apps compiladas listas para descargar.
- `build.ps1` (Windows), `build_mac.sh` (en un Mac), `build_linux.sh` (en Linux), `.github/workflows/build.yml` — compilación.

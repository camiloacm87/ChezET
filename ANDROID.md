# Plan para la app de Android

El proyecto ya está preparado para reutilizar todo en Android:

- **`server/`** es el núcleo (partida, sincronización, descubrimiento en la red) y expone
  `server.Start(puerto) (url, error)`, compatible con `gomobile bind`.
- **`server/web/`** es la interfaz (tablero, reglas, chat), igual en todas las plataformas.

## Cómo será la app

1. `gomobile bind -target=android ./server` genera `server.aar` (librería Android).
2. Un proyecto Android mínimo (Kotlin) con una sola `Activity` que:
   - llama a `Server.start(8765)` al abrirse,
   - muestra la URL devuelta en un `WebView` a pantalla completa,
   - pide los permisos `INTERNET`, `ACCESS_NETWORK_STATE`, `ACCESS_WIFI_STATE` y
     `CHANGE_WIFI_MULTICAST_STATE` (para recibir el broadcast de descubrimiento),
   - activa un `WifiManager.MulticastLock` mientras la app está abierta.
3. Resultado: un `.apk` que juega contra las versiones de Windows y Mac en la misma Wi-Fi.

## Qué se necesita instalar

- Android Studio (incluye el SDK y el NDK) y Java 17.
- `go install golang.org/x/mobile/cmd/gomobile@latest` y `gomobile init`.

## Detalle a ajustar

En Android 11+ las apps no pueden listar interfaces de red con `net.Interfaces()`.
Para Android habrá que obtener la IP desde Kotlin (`WifiManager`/`ConnectivityManager`) y
pasársela al núcleo; el resto del código funciona igual.

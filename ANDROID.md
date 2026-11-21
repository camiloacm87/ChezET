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

# Versión para Android 14+

El proyecto incluye un contenedor Android nativo en `packaging/android/` que reutiliza el servidor Go y la interfaz web existentes.

## Arquitectura

- Android Activity en Kotlin con `WebView`.
- Servidor Go incluido como `server.aar` mediante gomobile.
- API REST, SSE, chat y motor JavaScript reutilizados sin cambios.
- IP local obtenida con `ConnectivityManager`, no con `net.Interfaces()`.
- Multicast UDP protegido con `WifiManager.MulticastLock`.
- Unión manual por IP como alternativa si se bloquea el broadcast.

## Requisitos

- Android Studio: SDK API 35 y JDK 17.
- Go 1.22+.
- gomobile y Android NDK.
- Android mínimo soportado por el proyecto: API 26.
- Android objetivo recomendado: API 35 (Android 14+).

## Generar el AAR

Desde Windows, en la raíz del proyecto:

```powershell
.\packaging\android\build-aar.ps1
```

El script instala gomobile y genera:

```text
packaging/android/app/libs/server.aar
```

## Compilar

Abre la carpeta `packaging/android/` en Android Studio y ejecuta `Build > Make Project`.

Desde línea de comandos:

```powershell
cd packaging\android
gradle wrapper
.\gradlew.bat assembleDebug
```

El APK quedará en:

```text
packaging/android/app/build/outputs/apk/debug/app-debug.apk
```

## Configuración Android

El proyecto ya incluye:

- `minSdk = 26`.
- `targetSdk = 35`.
- Permisos de Internet y Wi-Fi.
- Permiso de cambio de bloqueo multicast.
- Tráfico HTTP local permitido para el servidor embebido.
- Cierre del servidor y liberación del multicast lock al destruir la actividad.

## Limitaciones conocidas

- Las redes corporativas pueden bloquear UDP broadcast aunque permita HTTP TCP.
- Android puede cambiar la IP al cambiar de red; la app actualiza la IP mediante `NetworkCallback`.
- Android 14 puede impedir el descubrimiento si no se mantiene la actividad activa o no existe un bloqueo multicast adecuado.
- Para máxima compatibilidad, la unión manual por `IP:8765` debe permanecer disponible.

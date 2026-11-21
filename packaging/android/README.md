# Ajedrez en Red para Android 14+

Proyecto Android nativo que ejecuta el servidor Go existente dentro de la aplicación y muestra `server/web` mediante `WebView`.

## Requisitos

- Android Studio con SDK API 35
- JDK 17
- Go 1.22+
- Android NDK
- Android 14 o posterior para el dispositivo objetivo

## Compilar el servidor Go como AAR

Desde Windows, en la raíz del proyecto:

```powershell
.\packaging\android\build-aar.ps1
```

El script instala gomobile, genera `packaging/android/app/libs/server.aar` y deja el proyecto listo para Gradle.

## Abrir y compilar

Abre la carpeta `packaging/android` desde Android Studio y pulsa `Build > Make Project`.

Luego instala el APK en un dispositivo/emulador con:

```powershell
cd packaging\android
gradle wrapper
gradlew assembleDebug
```

El APK quedará en `app/build/outputs/apk/debug/app-debug.apk`.

## Permisos y red

La app conserva un `WifiManager.MulticastLock` mientras esté activa y proporciona a Go la IP Wi-Fi obtenida mediante `ConnectivityManager`. La unión manual por IP permanece disponible si Android restringe el broadcast UDP.

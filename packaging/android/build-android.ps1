# Script de build completo para Android
# Genera server.aar (gomobile) y compila el APK con Gradle
# Uso: .\packaging\android\build-android.ps1

param(
    [string]$Go     = "",
    [string]$Flavor = "Debug"   # Debug | Release
)

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$androidDir  = (Resolve-Path $PSScriptRoot).Path

# ── Configurar entorno ────────────────────────────────────────────────────────
$env:ANDROID_HOME     = "$env:LOCALAPPDATA\Android\Sdk"
$env:JAVA_HOME        = "C:\Program Files\Android\Android Studio\jbr"
$env:ANDROID_NDK_HOME = (Get-ChildItem "$env:ANDROID_HOME\ndk" -ErrorAction SilentlyContinue |
                          Sort-Object Name | Select-Object -Last 1).FullName

Write-Host "ANDROID_HOME:     $env:ANDROID_HOME"
Write-Host "JAVA_HOME:        $env:JAVA_HOME"
Write-Host "ANDROID_NDK_HOME: $env:ANDROID_NDK_HOME"
Write-Host ""

# ── Paso 1: Generar server.aar ────────────────────────────────────────────────
Write-Host "=== Paso 1/3: Generando server.aar ===" -ForegroundColor Cyan
& (Join-Path $androidDir "build-aar.ps1") -Go $Go
if ($LASTEXITCODE -ne 0) { throw "Fallo al generar server.aar" }

# ── Paso 2: Compilar APK ──────────────────────────────────────────────────────
Write-Host "`n=== Paso 2/3: Compilando APK ($Flavor) ===" -ForegroundColor Cyan
Set-Location $androidDir

$task = if ($Flavor -eq "Release") { "assembleRelease" } else { "assembleDebug" }
& ".\gradlew.bat" $task
if ($LASTEXITCODE -ne 0) { throw "Gradle falló en la tarea $task" }

# ── Paso 3: Copiar a Ejecutables/Android ─────────────────────────────────────
Write-Host "`n=== Paso 3/3: Copiando APK a Ejecutables/Android ===" -ForegroundColor Cyan
$apkFlavor = $Flavor.ToLower()
$apkSrc    = Join-Path $androidDir "app\build\outputs\apk\$apkFlavor\app-$apkFlavor.apk"
$outDir    = Join-Path $projectRoot "Ejecutables\Android"
New-Item -ItemType Directory -Force $outDir | Out-Null
Copy-Item -Force $apkSrc (Join-Path $outDir "AjedrezEnRed.apk")

Write-Host "`n✅ APK listo en: Ejecutables\Android\AjedrezEnRed.apk" -ForegroundColor Green

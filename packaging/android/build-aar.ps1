param([string]$Go = "go")

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$androidDir  = (Resolve-Path $PSScriptRoot).Path
$libsDir     = Join-Path $androidDir "app\libs"

# ── 1. Verificar go ───────────────────────────────────────────────────────────
& $Go version
if ($LASTEXITCODE -ne 0) { throw "Go no está disponible. Asegúrate de tener Go instalado y en el PATH." }

# ── 2. Instalar gomobile ──────────────────────────────────────────────────────
Write-Host "`n[1/4] Instalando gomobile..."
& $Go install golang.org/x/mobile/cmd/gomobile@latest
if ($LASTEXITCODE -ne 0) { throw "No se pudo instalar gomobile." }

$gopath = (& $Go env GOPATH).Trim()
$gomobile = Join-Path $gopath "bin\gomobile.exe"
if (-not (Test-Path $gomobile)) { throw "gomobile no encontrado en $gomobile" }

# ── 3. Inicializar gomobile ───────────────────────────────────────────────────
Write-Host "`n[2/4] Inicializando gomobile con Android SDK y NDK..."

# Obtener rutas cortas 8.3 de Windows para evitar problemas con espacios en 'ASUS VIVOBOOK'
$fso = New-Object -ComObject Scripting.FileSystemObject
$sdkRaw = "$env:LOCALAPPDATA\Android\Sdk"
$ndkLatest = (Get-ChildItem "$sdkRaw\ndk" -ErrorAction SilentlyContinue | Sort-Object Name | Select-Object -Last 1).FullName

if (-not $ndkLatest) { throw "No se encontró Android NDK en $sdkRaw\ndk" }

$env:ANDROID_HOME     = $fso.GetFolder($sdkRaw).ShortPath
$env:ANDROID_NDK_HOME = $fso.GetFolder($ndkLatest).ShortPath

Write-Host "ANDROID_HOME:     $env:ANDROID_HOME"
Write-Host "ANDROID_NDK_HOME: $env:ANDROID_NDK_HOME"

& $gomobile init
if ($LASTEXITCODE -ne 0) { throw "gomobile init falló." }

# ── 4. Generar server.aar ─────────────────────────────────────────────────────
Write-Host "`n[3/4] Generando server.aar..."
New-Item -ItemType Directory -Force $libsDir | Out-Null
$targetAar = Join-Path $libsDir "server.aar"

$androidGoMod = Join-Path $androidDir "gomod"
Set-Location $androidGoMod
& $Go mod tidy
& $gomobile bind -target=android -androidapi 26 -javapkg com.chezred.go -o $targetAar .
if ($LASTEXITCODE -ne 0) { throw "No se pudo generar server.aar." }

Write-Host "`n[4/4] Verificando resultado..."
if (-not (Test-Path $targetAar)) { throw "No se encontró $targetAar" }

Write-Host "`n✅ Listo: packaging\android\app\libs\server.aar generado con éxito."

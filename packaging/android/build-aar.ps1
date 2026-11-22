param([string]$Go = "")

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$androidDir  = (Resolve-Path $PSScriptRoot).Path
$libsDir     = Join-Path $androidDir "app\libs"

# ── 1. Localizar go.exe ───────────────────────────────────────────────────────
if ($Go -eq "") {
    # Buscar go en rutas habituales si no está en el PATH
    $candidates = @(
        "go",
        "C:\Program Files\Go\bin\go.exe",
        "C:\Go\bin\go.exe",
        "$env:USERPROFILE\go\bin\go.exe"
    )
    foreach ($c in $candidates) {
        if (Get-Command $c -ErrorAction SilentlyContinue) { $Go = $c; break }
    }
    if ($Go -eq "") {
        throw "No se encontró go.exe. Instala Go 1.22+ desde https://go.dev/dl/ y agrega su bin al PATH."
    }
}

Write-Host "Usando Go: $Go"
& $Go version
if ($LASTEXITCODE -ne 0) { throw "Go no está disponible." }

# ── 2. Instalar gomobile ──────────────────────────────────────────────────────
Write-Host "`n[1/4] Instalando gomobile..."
& $Go install golang.org/x/mobile/cmd/gomobile@latest
if ($LASTEXITCODE -ne 0) { throw "No se pudo instalar gomobile." }

# Localizar gomobile en GOPATH/bin
$gopath = (& $Go env GOPATH).Trim()
$gomobile = Join-Path $gopath "bin\gomobile.exe"
if (-not (Test-Path $gomobile)) { throw "gomobile no encontrado en $gomobile" }

# ── 3. Inicializar gomobile (descarga las libs de Android) ───────────────────
Write-Host "`n[2/4] Inicializando gomobile (descarga Android libs)..."
$env:ANDROID_HOME     = "$env:LOCALAPPDATA\Android\Sdk"
$env:ANDROID_NDK_HOME = (Get-ChildItem "$env:ANDROID_HOME\ndk" | Sort-Object Name | Select-Object -Last 1).FullName
Write-Host "ANDROID_HOME:     $env:ANDROID_HOME"
Write-Host "ANDROID_NDK_HOME: $env:ANDROID_NDK_HOME"
& $gomobile init
if ($LASTEXITCODE -ne 0) { throw "gomobile init falló." }

# ── 4. Generar server.aar ─────────────────────────────────────────────────────
Write-Host "`n[3/4] Generando server.aar desde ./server..."
New-Item -ItemType Directory -Force $libsDir | Out-Null

Set-Location $projectRoot

# Usamos el go.mod del sub-módulo android para evitar las deps de escritorio
$androidGoMod = Join-Path $androidDir "gomod"
if (Test-Path $androidGoMod) {
    Set-Location $androidGoMod
    & $Go mod tidy
    & $gomobile bind -target=android -javapkg=com.chezred.go .
} else {
    # Fallback: compilar desde la raíz con tag android para excluir deps de escritorio
    & $gomobile bind -target=android -javapkg=com.chezred.go -tags android ./server
}
if ($LASTEXITCODE -ne 0) { throw "No se pudo generar server.aar." }

# ── 5. Mover el AAR al lugar esperado por Gradle ──────────────────────────────
Write-Host "`n[4/4] Copiando server.aar a app/libs/..."
$aarSrc = if (Test-Path $androidGoMod) {
    Join-Path $androidGoMod "server.aar"
} else {
    Join-Path $projectRoot "server.aar"
}
Move-Item -Force $aarSrc (Join-Path $libsDir "server.aar")

Write-Host "`n✅ Listo: packaging\android\app\libs\server.aar generado."

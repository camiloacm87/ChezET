param([string]$Go = "go")

$ErrorActionPreference = "Stop"
$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
$androidDir = (Resolve-Path $PSScriptRoot).Path
Set-Location $projectRoot

& $Go version
if ($LASTEXITCODE -ne 0) { throw "Go no está disponible. Instala Go 1.22 o posterior." }

& $Go install golang.org/x/mobile/cmd/gomobile@latest
if ($LASTEXITCODE -ne 0) { throw "No se pudo instalar gomobile." }

& $Go run golang.org/x/mobile/cmd/gomobile init
if ($LASTEXITCODE -ne 0) { throw "No se pudo inicializar gomobile." }

$libsDir = Join-Path $androidDir "app\libs"
New-Item -ItemType Directory -Force $libsDir | Out-Null
& $Go run golang.org/x/mobile/cmd/gomobile bind -target=android -javapkg=com.chezred.go ./server
if ($LASTEXITCODE -ne 0) { throw "No se pudo generar server.aar." }

Move-Item -Force (Join-Path $projectRoot "server.aar") (Join-Path $libsDir "server.aar")
Write-Host "Generado: packaging\android\app\libs\server.aar"

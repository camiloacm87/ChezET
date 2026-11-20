# Compila la app de Windows de Ajedrez en Red:
#   Ejecutables\Windows\AjedrezEnRed.exe  (ventana propia con WebView2, icono incluido)
# La app de Mac se compila en un Mac con build_mac.sh, o automáticamente en GitHub Actions.
#
# Uso:  .\build.ps1                (usa "go" del PATH)
#       .\build.ps1 -Go C:\ruta\go\bin\go.exe
param([string]$Go = "go")

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$winDir = Join-Path $PSScriptRoot "Ejecutables\Windows"
New-Item -ItemType Directory -Force $winDir | Out-Null

# Icono + manifiesto del .exe
& $Go run ./tools/mkrsrc
if ($LASTEXITCODE -ne 0) { throw "falló la generación de recursos" }

$env:CGO_ENABLED = "0"; $env:GOOS = "windows"; $env:GOARCH = "amd64"
$exe = Join-Path $winDir "AjedrezEnRed.exe"
Write-Host "Compilando -> $exe"
& $Go build -trimpath -ldflags "-s -w -H windowsgui" -o $exe .
$code = $LASTEXITCODE
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
if ($code -ne 0) { throw "falló la compilación" }

Write-Host "Listo: $exe ($([math]::Round((Get-Item $exe).Length/1MB,1)) MB)"

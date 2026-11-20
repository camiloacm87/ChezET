# Compila Ajedrez en Red desde Windows.
#  - Ejecutables\Windows\AjedrezEnRed.exe             App de Windows con ventana propia (WebView2).
#  - Ejecutables\Mac\AjedrezEnRed-Mac-sin-ventana.zip  Versión de Mac provisional que usa el navegador
#    (la app de Mac con ventana propia se compila en un Mac con build_mac.sh, o en GitHub Actions).
#
# Uso:  .\build.ps1                (usa "go" del PATH)
#       .\build.ps1 -Go C:\ruta\go\bin\go.exe
param([string]$Go = "go")

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot
$out = Join-Path $PSScriptRoot "Ejecutables"
$winDir = Join-Path $out "Windows"
$macDir = Join-Path $out "Mac"
$stage = Join-Path $out "_tmp\AjedrezEnRed-Mac"
New-Item -ItemType Directory -Force $winDir, $macDir, $stage | Out-Null

$env:CGO_ENABLED = "0"

function Build($os, $arch, $out, $ldflags) {
    $env:GOOS = $os; $env:GOARCH = $arch
    Write-Host "Compilando $os/$arch -> $out"
    & $Go build -trimpath -ldflags $ldflags -o $out .
    if ($LASTEXITCODE -ne 0) { throw "falló la compilación $os/$arch" }
}

# Icono + manifiesto del .exe
Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
& $Go run ./tools/mkrsrc
if ($LASTEXITCODE -ne 0) { throw "falló la generación de recursos" }

Build "windows" "amd64" (Join-Path $winDir "AjedrezEnRed.exe") "-s -w -H windowsgui"
Build "darwin"  "arm64" (Join-Path $stage "ajedrez-mac-applesilicon") "-s -w"
Build "darwin"  "amd64" (Join-Path $stage "ajedrez-mac-intel") "-s -w"

Remove-Item Env:GOOS, Env:GOARCH
Copy-Item "packaging\mac\Abrir Ajedrez.command", "packaging\mac\LEEME.txt" $stage -Force

$zip = Join-Path $macDir "AjedrezEnRed-Mac-sin-ventana.zip"
& $Go run ./tools/mkzip $zip "AjedrezEnRed-Mac" `
    (Join-Path $stage "Abrir Ajedrez.command") `
    (Join-Path $stage "ajedrez-mac-applesilicon") `
    (Join-Path $stage "ajedrez-mac-intel") `
    (Join-Path $stage "LEEME.txt")
if ($LASTEXITCODE -ne 0) { throw "falló el empaquetado para Mac" }
Remove-Item (Join-Path $out "_tmp") -Recurse -Force

Write-Host ""
Write-Host "Listo:"
Get-ChildItem $out -Recurse -File | Format-Table @{n="Archivo"; e={$_.FullName.Substring($out.Length + 1)}}, @{n="MB"; e={[math]::Round($_.Length/1MB,1)}} -AutoSize

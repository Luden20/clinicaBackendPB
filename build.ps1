<#
.SYNOPSIS
  Compila clinicaBackendPB en esta máquina (Windows) y lo publica en la rama release.

.DESCRIPTION
  1. go build para linux-amd64 a dist/pocketbase (las migraciones Go van dentro).
  2. Hace push de main.
  3. Crea la rama release = main + dist/ (un solo commit, se sobrescribe en cada
     build para que el repo no crezca) y hace push --force.
  No despliega nada. En el VPS: ./deploy.sh

.PARAMETER NoPush
  Compila a dist/ sin push.

.PARAMETER Help
  Muestra esta ayuda y sale (también: -h, -?).

.EXAMPLE
  .\build.ps1           # compilar y publicar release
  .\build.ps1 -NoPush   # solo compilar
#>
[CmdletBinding()]
param(
    [switch] $NoPush,
    [Alias('h')]
    [switch] $Help
)

if ($Help) { Get-Help $PSCommandPath -Detailed; return }

$ErrorActionPreference = 'Stop'
$PSNativeCommandUseErrorActionPreference = $true
Set-Location $PSScriptRoot

$Dist = Join-Path $PSScriptRoot 'dist'
$Stamp = Get-Date -Format 'yyyyMMdd-HHmm'

function Step($msg) { Write-Host "==> $msg" -ForegroundColor Cyan }

if (-not $NoPush) {
    $dirty = git status --porcelain
    if ($dirty) { throw "Hay cambios sin commit. Haz commit primero:`n$($dirty -join "`n")" }
    if ((git branch --show-current) -ne 'main') { throw 'La release se publica desde main.' }
}

# ---------- Build ----------
Step 'go build linux-amd64'
if (Test-Path $Dist) { Remove-Item $Dist -Recurse -Force }
New-Item -ItemType Directory -Path $Dist | Out-Null
$env:CGO_ENABLED = '0'; $env:GOOS = 'linux'; $env:GOARCH = 'amd64'
try {
    go build -trimpath -ldflags '-s -w' -o (Join-Path $Dist 'pocketbase') .
}
finally {
    Remove-Item env:CGO_ENABLED, env:GOOS, env:GOARCH
}
$sha = git rev-parse --short HEAD
"$sha $Stamp" | Set-Content -NoNewline (Join-Path $Dist 'build-info.txt')
Write-Host "   $(Get-Content (Join-Path $Dist 'build-info.txt'))"

if ($NoPush) { Step 'Compilado en dist/ (sin push)'; return }

# ---------- main ----------
Step 'Push de main'
git push -q origin main

# ---------- release: main + dist/ ----------
Step 'Publicando rama release'
$index = Join-Path (Resolve-Path .git) 'release-index'
$env:GIT_INDEX_FILE = $index
try {
    git read-tree HEAD
    git add -f dist
    git update-index --chmod=+x dist/pocketbase
    $tree = git write-tree
}
finally {
    Remove-Item env:GIT_INDEX_FILE
    Remove-Item $index -ErrorAction SilentlyContinue
}
$commit = "release $Stamp (main $sha)" | git commit-tree $tree
git push -q --force origin "${commit}:refs/heads/release"

Step "Listo: release $Stamp. En el VPS: ./deploy.sh"

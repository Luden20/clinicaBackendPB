<#
.SYNOPSIS
  Compila clinicaBackendPB en esta máquina (Windows) y lo despliega en el VPS.

.DESCRIPTION
  1. go build para linux-amd64 (las migraciones Go van dentro del binario).
  2. Arma dist/: pocketbase, deploy.sh, clinica-backend.service, build-info.txt.
  3. Sube dist/ por scp a $VPS_DIR/incoming y corre deploy.sh por ssh, que hace
     backup de pb_data, migrate up, reinicio del servicio, healthcheck y
     rollback automático si algo falla.

.PARAMETER NoDeploy
  Compila a dist/ sin subir nada.

.PARAMETER Rollback
  No compila: en el VPS vuelve al binario anterior (pocketbase.prev).

.PARAMETER RollbackData
  Como -Rollback, pero además restaura pb_data del último backup.
  Se pierde lo escrito en la base desde el último deploy.

.PARAMETER Help
  Muestra esta ayuda y sale (también: -h, -?).

.EXAMPLE
  .\build.ps1                 # compilar y desplegar
  .\build.ps1 -NoDeploy       # solo compilar
  .\build.ps1 -Rollback       # volver al binario anterior
  .\build.ps1 -RollbackData   # binario y base anteriores
#>
[CmdletBinding()]
param(
    [switch] $NoDeploy,
    [switch] $Rollback,
    [switch] $RollbackData,
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

# ---------- Configuración local ----------
if (Test-Path .env.build) {
    Get-Content .env.build | Where-Object { $_ -match '^\s*[^#\s][^=]*=' } | ForEach-Object {
        $k, $v = $_ -split '=', 2
        Set-Item -Path "env:$($k.Trim())" -Value $v.Trim()
    }
}
if (-not $NoDeploy) {
    if (-not (Test-Path .env.build)) { throw 'Falta .env.build (copia .env.build.example y llénalo).' }
    foreach ($k in 'VPS_HOST', 'VPS_USER') {
        if (-not (Get-Item "env:$k" -ErrorAction SilentlyContinue).Value) { throw "Falta $k en .env.build." }
    }
}
$VpsDir = if ($env:VPS_DIR) { $env:VPS_DIR } else { '/opt/clinica-backend' }
$Target = "$($env:VPS_USER)@$($env:VPS_HOST)"
$SshArgs = @('-p', $(if ($env:VPS_PORT) { $env:VPS_PORT } else { '22' }))
$ScpArgs = @('-P', $SshArgs[1])
if ($env:VPS_SSH_KEY) { $SshArgs += '-i', $env:VPS_SSH_KEY; $ScpArgs += '-i', $env:VPS_SSH_KEY }

# ---------- Rollback ----------
if ($Rollback -or $RollbackData) {
    $flag = if ($RollbackData) { '--rollback-data' } else { '--rollback' }
    Step "Rollback en $Target ($flag)"
    ssh @SshArgs $Target "APP_DIR='$VpsDir' bash '$VpsDir/deploy.sh' $flag"
    return
}

# ---------- Build ----------
$dirty = git status --porcelain
if ($dirty) { Write-Host "Aviso: hay cambios sin commit, se compilan igual:`n$($dirty -join "`n")" -ForegroundColor Yellow }

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

# Los scripts van con LF aunque git los haya sacado con CRLF.
foreach ($f in 'deploy.sh', 'clinica-backend.service') {
    $text = (Get-Content (Join-Path 'deploy' $f) -Raw) -replace "`r`n", "`n"
    [IO.File]::WriteAllText((Join-Path $Dist $f), $text)
}
$sha = git rev-parse --short HEAD
$suffix = if ($dirty) { '-dirty' } else { '' }
"$sha$suffix $Stamp" | Set-Content -NoNewline (Join-Path $Dist 'build-info.txt')
Write-Host "   $(Get-Content (Join-Path $Dist 'build-info.txt'))"

if ($NoDeploy) { Step 'Compilado en dist/ (sin deploy)'; return }

# ---------- Deploy ----------
Step "Subiendo a ${Target}:$VpsDir/incoming"
ssh @SshArgs $Target "mkdir -p '$VpsDir/incoming'"
scp @ScpArgs (Get-ChildItem $Dist).FullName "${Target}:$VpsDir/incoming/"

Step 'Ejecutando deploy.sh en el VPS'
ssh @SshArgs $Target "APP_DIR='$VpsDir' bash '$VpsDir/incoming/deploy.sh'"

Step "Listo: $sha$suffix desplegado en $($env:VPS_HOST)"

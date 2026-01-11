Param(
  [string]$EthminerSrc = "",
  [string]$GethSrc = "",
  [string]$GenesisSrc = ""
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Dist = Join-Path $Root "dist"

New-Item -ItemType Directory -Force -Path $Dist | Out-Null

Write-Host "[1/3] Building GUI..."
Push-Location $Root
go mod tidy
go build -trimpath -ldflags="-H=windowsgui -s -w" -o (Join-Path $Dist "OlivetumMiner.exe") .
Pop-Location

if ($EthminerSrc -ne "") {
  if (!(Test-Path $EthminerSrc)) {
    throw "ethminer binary not found at: $EthminerSrc"
  }
  Write-Host "[2/3] Copying ethminer..."
  Copy-Item -Force $EthminerSrc (Join-Path $Dist "ethminer.exe")
} else {
  Write-Host "[2/3] Skipping ethminer copy (pass -EthminerSrc to bundle it)."
}

if ($GethSrc -ne "") {
  if (!(Test-Path $GethSrc)) {
    throw "geth binary not found at: $GethSrc"
  }
  Write-Host "[2/3] Copying geth..."
  Copy-Item -Force $GethSrc (Join-Path $Dist "geth.exe")
} else {
  Write-Host "[2/3] Skipping geth copy (pass -GethSrc to bundle it)."
}

if ($GenesisSrc -eq "") {
  $GenesisSrc = Join-Path $Root "assets\olivetum_pow_genesis.json"
}
if (Test-Path $GenesisSrc) {
  Write-Host "[2/3] Copying genesis..."
  Copy-Item -Force $GenesisSrc (Join-Path $Dist "olivetum_pow_genesis.json")
} else {
  Write-Host "[2/3] Skipping genesis copy (file not found): $GenesisSrc"
}

Write-Host "[3/3] Creating zip..."
$ZipPath = Join-Path $Dist "OlivetumMiner-windows-x86_64.zip"
if (Test-Path $ZipPath) { Remove-Item -Force $ZipPath }

$Files = @((Join-Path $Dist "OlivetumMiner.exe"))
if (Test-Path (Join-Path $Dist "ethminer.exe")) {
  $Files += (Join-Path $Dist "ethminer.exe")
}
if (Test-Path (Join-Path $Dist "geth.exe")) {
  $Files += (Join-Path $Dist "geth.exe")
}
if (Test-Path (Join-Path $Dist "olivetum_pow_genesis.json")) {
  $Files += (Join-Path $Dist "olivetum_pow_genesis.json")
}

Compress-Archive -Force -Path $Files -DestinationPath $ZipPath
Write-Host "Done: $ZipPath"

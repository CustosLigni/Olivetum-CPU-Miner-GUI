Param(
  [string]$EthminerSrc = ""
)

$ErrorActionPreference = "Stop"

$Root = Split-Path -Parent $MyInvocation.MyCommand.Path
$Dist = Join-Path $Root "dist"

New-Item -ItemType Directory -Force -Path $Dist | Out-Null

Write-Host "[1/3] Building GUI..."
Push-Location $Root
go mod tidy
go build -ldflags="-H=windowsgui" -o (Join-Path $Dist "OlivetumMiner.exe") .
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

Write-Host "[3/3] Creating zip..."
$ZipPath = Join-Path $Dist "OlivetumMiner-windows-x86_64.zip"
if (Test-Path $ZipPath) { Remove-Item -Force $ZipPath }

$Files = @((Join-Path $Dist "OlivetumMiner.exe"))
if (Test-Path (Join-Path $Dist "ethminer.exe")) {
  $Files += (Join-Path $Dist "ethminer.exe")
}

Compress-Archive -Force -Path $Files -DestinationPath $ZipPath
Write-Host "Done: $ZipPath"


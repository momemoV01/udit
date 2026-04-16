# Optional: -NoCompletion or $env:UDIT_NO_COMPLETION = "1" to skip the
# shell-completion auto-install at the end.
# Optional: -NoChecksum or $env:UDIT_NO_CHECKSUM = "1" to skip checksum
# verification after download.
param(
    [switch]$NoCompletion,
    [switch]$NoChecksum
)

$ErrorActionPreference = "Stop"
$skipCompletion = $NoCompletion -or ($env:UDIT_NO_COMPLETION -eq "1")
$skipChecksum = $NoChecksum -or ($env:UDIT_NO_CHECKSUM -eq "1")

$repo = "momemoV01/udit"
$installDir = "$env:LOCALAPPDATA\udit"
$exe = "$installDir\udit.exe"
$binary = "udit-windows-amd64.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$url = "https://github.com/$repo/releases/latest/download/$binary"
Write-Host "Downloading udit for windows/amd64..."
Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing

# Checksum verification (skip with -NoChecksum or $env:UDIT_NO_CHECKSUM = "1").
if (-not $skipChecksum) {
    $sumsUrl = "https://github.com/$repo/releases/latest/download/SHA256SUMS.txt"
    try {
        $sums = (Invoke-WebRequest -Uri $sumsUrl -UseBasicParsing).Content
        $line = ($sums -split "`n" | Where-Object { $_ -match $binary })
        if ($line) {
            $expected = ($line -split '\s+')[0].ToLower()
            $actual = (Get-FileHash $exe -Algorithm SHA256).Hash.ToLower()
            if ($expected -ne $actual) {
                Remove-Item $exe -Force
                throw "Checksum mismatch! Expected: $expected, Got: $actual"
            }
            Write-Host "Checksum verified."
        } else {
            Write-Host "Warning: no checksum entry for $binary (skipping verification)."
        }
    } catch {
        if ($_.Exception.Message -match "Checksum mismatch") { throw }
        Write-Host "Warning: could not download checksums (skipping verification)."
    }
}

$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$installDir;$userPath", "User")
    $env:Path = "$installDir;$env:Path"
    Write-Host "Added $installDir to PATH (restart shell to apply)"
}

Write-Host "Installed udit to $exe"
& $exe version

# Auto-install PowerShell completion. Best-effort.
if (-not $skipCompletion) {
    try {
        & $exe completion install
    } catch {
        Write-Host "Note: shell completion install skipped (run ``udit completion install`` later)."
    }
}

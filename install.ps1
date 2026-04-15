# Optional: -NoCompletion or $env:UDIT_NO_COMPLETION = "1" to skip the
# shell-completion auto-install at the end.
param(
    [switch]$NoCompletion
)

$ErrorActionPreference = "Stop"
$skipCompletion = $NoCompletion -or ($env:UDIT_NO_COMPLETION -eq "1")

$repo = "momemoV01/udit"
$installDir = "$env:LOCALAPPDATA\udit"
$exe = "$installDir\udit.exe"

New-Item -ItemType Directory -Force -Path $installDir | Out-Null

$url = "https://github.com/$repo/releases/latest/download/udit-windows-amd64.exe"
Write-Host "Downloading udit for windows/amd64..."
Invoke-WebRequest -Uri $url -OutFile $exe -UseBasicParsing

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

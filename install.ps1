$ErrorActionPreference = "Stop"

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

# Build ngw.exe on Windows (needs Go 1.24+): .\build.ps1
$ErrorActionPreference = "Stop"
New-Item -ItemType Directory -Force dist | Out-Null
go build -trimpath -ldflags "-s -w -H windowsgui" -o dist\ngw.exe .\cmd\ngw
Write-Host "built dist\ngw.exe"

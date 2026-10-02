param(
    [string]$ClientId,
    [System.Security.SecureString]$ClientSecret
)
$ErrorActionPreference = 'Stop'
$source = Join-Path $PSScriptRoot 'multica.exe'
if (!(Test-Path -LiteralPath $source)) { throw 'Extract the complete client ZIP before running this installer.' }
if (Get-Process -Name multica -ErrorAction SilentlyContinue) {
    throw 'Stop Multica on this computer before replacing the client, then rerun this installer.'
}
$bin = Join-Path $env:USERPROFILE '.multica\bin'
New-Item -ItemType Directory -Force -Path $bin | Out-Null
$target = Join-Path $bin 'multica.exe'
if (Test-Path -LiteralPath $target) {
    Copy-Item -LiteralPath $target -Destination ($target + '.before-cloudflare-' + (Get-Date -Format yyyyMMdd-HHmmss))
}
Copy-Item -LiteralPath $source -Destination $target -Force
foreach ($setting in @(
    @('server_url', 'https://multica.edgeofglory.dev'),
    @('app_url', 'https://multica.edgeofglory.dev'),
    @('disable_auto_update', 'true')
)) {
    & $target config set $setting[0] $setting[1]
    if ($LASTEXITCODE -ne 0) { throw 'Multica client configuration failed.' }
}
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if (($userPath -split ';') -notcontains $bin) {
    [Environment]::SetEnvironmentVariable('Path', (([string]$userPath).TrimEnd(';') + ';' + $bin), 'User')
}
$env:Path = $bin + ';' + $env:Path
& (Join-Path $PSScriptRoot 'configure-cloudflare-access.ps1') -ClientId $ClientId -ClientSecret $ClientSecret
Write-Host 'Client installed. Existing login and workspace settings are preserved. For an existing computer, run multica daemon start. For a new computer, continue with Add computer in Multica.'

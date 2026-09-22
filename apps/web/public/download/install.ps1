# Multica Cloudflare installer
param(
    [Parameter(Mandatory=$true)][string]$ClientId,
    [Parameter(Mandatory=$true)][System.Security.SecureString]$ClientSecret
)
$ErrorActionPreference = 'Stop'
if (![Environment]::Is64BitOperatingSystem -or $env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {
    throw 'This installer requires Windows x64.'
}
if (Get-Process -Name multica -ErrorAction SilentlyContinue) {
    throw 'Stop Multica on this computer and run the install command again.'
}
$directory = Join-Path $env:USERPROFILE ('.multica\install-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $directory | Out-Null
$zip = Join-Path $directory 'client.zip'
$headers = @{
    'CF-Access-Client-Id' = $ClientId.Trim()
    'CF-Access-Client-Secret' = [System.Net.NetworkCredential]::new('', $ClientSecret).Password.Trim()
}
try {
    # Do not forward shared credentials to any redirect destination.
    Invoke-WebRequest -UseBasicParsing -MaximumRedirection 0 -Headers $headers -Uri 'https://multica.edgeofglory.dev/download/multica-cloudflare-windows-amd64.zip' -OutFile $zip
} catch {
    throw 'Could not download the Multica package. Check the Cloudflare service token and Service Auth policy for /download/*.'
} finally {
    $headers.Clear()
}
Expand-Archive -LiteralPath $zip -DestinationPath $directory
$expected = ((Get-Content -LiteralPath (Join-Path $directory 'SHA256SUMS.txt') -Raw).Trim() -split '\s+')[0]
$actual = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $directory 'multica.exe')).Hash
if ($expected -notmatch '^[a-fA-F0-9]{64}$' -or $actual -ne $expected) {
    throw 'The downloaded Multica executable failed its checksum check.'
}
& (Join-Path $directory 'install-cloudflare-client.ps1') -ClientId $ClientId -ClientSecret $ClientSecret
Write-Host 'Next: run the second setup command shown in Add computer.'

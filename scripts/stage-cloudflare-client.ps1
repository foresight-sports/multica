param([string]$PackagePath = (Join-Path $PSScriptRoot '..\..\releases\multica-cloudflare-windows-amd64.zip'))
$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $PackagePath).Path)
try {
    $allowed = @('multica.exe', 'install-cloudflare-client.ps1', 'configure-cloudflare-access.ps1', 'CLOUDFLARE-CLIENT.md', 'SHA256SUMS.txt')
    $entries = @($archive.Entries | ForEach-Object { $_.FullName })
    if ($entries.Count -ne $allowed.Count -or @($entries | Where-Object { $_ -notin $allowed }).Count -gt 0 -or @($allowed | Where-Object { $_ -notin $entries }).Count -gt 0) {
        throw 'Unexpected client ZIP contents. Stage only the release package with no credential files.'
    }
} finally {
    $archive.Dispose()
}
$destination = Join-Path $PSScriptRoot '..\apps\web\public\download'
New-Item -ItemType Directory -Force -Path $destination | Out-Null
Copy-Item -LiteralPath $PackagePath -Destination (Join-Path $destination 'multica-cloudflare-windows-amd64.zip') -Force
Write-Host 'Client staged. Rebuild the web image to publish it with the Add computer instructions.'

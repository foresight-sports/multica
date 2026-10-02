param(
    [Parameter(Mandatory=$true)][ValidatePattern('^\d+\.\d+\.\d+-foresight\.\d+$')][string]$Version,
    [Parameter(Mandatory=$true)][hashtable]$Artifacts,
    [string]$ReleaseDirectory = (Join-Path $PSScriptRoot '..\daemon-releases')
)
$ErrorActionPreference = 'Stop'
if ($Artifacts.Count -eq 0) { throw 'At least one artifact is required.' }
$root = [IO.Path]::GetFullPath($ReleaseDirectory)
New-Item -ItemType Directory -Force -Path $root | Out-Null
$manifestPath = Join-Path $root 'manifest.json'
if (Test-Path -LiteralPath $manifestPath) {
    $previous = Get-Content -LiteralPath $manifestPath -Raw | ConvertFrom-Json
    $oldParts = $previous.version -split '[.-]' | Where-Object { $_ -ne 'foresight' } | ForEach-Object { [int]$_ }
    $newParts = $Version -split '[.-]' | Where-Object { $_ -ne 'foresight' } | ForEach-Object { [int]$_ }
    for ($i=0; $i -lt 4; $i++) {
        if ($newParts[$i] -lt $oldParts[$i]) { throw 'Refusing to publish a downgrade.' }
        if ($newParts[$i] -gt $oldParts[$i]) { break }
    }
}
$release = Join-Path $root $Version
New-Item -ItemType Directory -Force -Path $release | Out-Null
$assets = @{}
foreach ($platform in $Artifacts.Keys) {
    if ($platform -notmatch '^(windows|linux|darwin)-(amd64|arm64)$') { throw "Unsupported platform: $platform" }
    $source = Get-Item -LiteralPath $Artifacts[$platform]
    if ($source.PSIsContainer -or $source.Length -le 0 -or $source.Length -gt 150MB) { throw 'Invalid artifact size.' }
    $hash = (Get-FileHash -LiteralPath $source.FullName -Algorithm SHA256).Hash.ToLowerInvariant()
    $filename = 'multica-' + $platform
    if ($platform.StartsWith('windows-')) { $filename += '.exe' }
    $target = Join-Path $release $filename
    if (Test-Path -LiteralPath $target) {
        if ((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash.ToLowerInvariant() -ne $hash) { throw 'Published versions are immutable. Choose a new version.' }
    } else { Copy-Item -LiteralPath $source.FullName -Destination $target }
    $assets[$platform] = @{sha256=$hash;size=$source.Length}
}
$manifest = @{version=$Version;assets=$assets} | ConvertTo-Json -Depth 5
$tempManifest = Join-Path $root ('manifest-' + [guid]::NewGuid().ToString('N') + '.tmp')
[IO.File]::WriteAllText($tempManifest, $manifest, [Text.UTF8Encoding]::new($false))
Move-Item -LiteralPath $tempManifest -Destination $manifestPath -Force
Write-Host "Published daemon $Version ($($assets.Count) platforms)."

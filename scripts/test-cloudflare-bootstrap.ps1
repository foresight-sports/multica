$ErrorActionPreference = 'Stop'
$repo = Split-Path $PSScriptRoot -Parent
$fixture = Join-Path ([IO.Path]::GetTempPath()) ('multica-bootstrap-test-' + [guid]::NewGuid().ToString('N'))
$payload = Join-Path $fixture 'payload'
New-Item -ItemType Directory -Path $payload -Force | Out-Null
[IO.File]::WriteAllText((Join-Path $payload 'multica.exe'), 'test binary')
$hash = (Get-FileHash -LiteralPath (Join-Path $payload 'multica.exe')).Hash
[IO.File]::WriteAllText((Join-Path $payload 'SHA256SUMS.txt'), $hash + '  multica.exe')
$stub = @'
param([string]$ClientId, [Security.SecureString]$ClientSecret)
if ($ClientId -ne 'test-id' -or [Net.NetworkCredential]::new('', $ClientSecret).Password -ne 'test-secret') { throw 'Credentials did not reach installer' }
[IO.File]::WriteAllText((Join-Path $env:USERPROFILE 'installed.txt'), 'ok')
'@
[IO.File]::WriteAllText((Join-Path $payload 'install-cloudflare-client.ps1'), $stub)
$script:fixtureArchive = Join-Path $fixture 'fixture.zip'
Compress-Archive -Path ($payload + '\*') -DestinationPath $script:fixtureArchive
$bootstrap = [IO.File]::ReadAllText((Join-Path $repo 'apps/web/public/download/install.ps1'))
$command = [IO.File]::ReadAllText((Join-Path $repo 'server/internal/handler/runtime_install_command.ps1')).Replace('{{CLIENT_ID}}', "'test-id'").Replace('{{CLIENT_SECRET}}', "'test-secret'")
if (!$command) { throw 'Copyable command not found' }
function Read-Host {
    param($Prompt, [switch]$AsSecureString)
    if ($AsSecureString) { return ConvertTo-SecureString 'test-secret' -AsPlainText -Force }
    return 'test-id'
}
function Set-ExecutionPolicy { param($ExecutionPolicy, $Scope, [switch]$Force) }
function Get-Process { param($Name, $ErrorAction) }
function Invoke-WebRequest {
    param($Uri, $Headers, $MaximumRedirection, $OutFile, [switch]$UseBasicParsing)
    if ($MaximumRedirection -ne 0) { throw 'Redirects must be disabled' }
    if ($Headers['CF-Access-Client-Id'] -ne 'test-id' -or $Headers['CF-Access-Client-Secret'] -ne 'test-secret') { throw 'Missing Access headers' }
    if ($Uri -eq 'https://multica.edgeofglory.dev/download/install.ps1') {
        if ($script:serveHTML) { return @{ Content = '<html>Cloudflare login</html>' } }
        return @{ Content = [Text.Encoding]::UTF8.GetBytes($bootstrap) }
    }
    if ($Uri -ne 'https://multica.edgeofglory.dev/download/multica-cloudflare-windows-amd64.zip') { throw 'Unexpected download origin' }
    Copy-Item -LiteralPath $script:fixtureArchive -Destination $OutFile
}
$savedProfile = $env:USERPROFILE
try {
    $env:USERPROFILE = $fixture
    & ([scriptblock]::Create($command))
    if (!(Test-Path -LiteralPath (Join-Path $fixture 'installed.txt'))) { throw 'Installer did not run' }
    $script:serveHTML = $true
    $rejected = $false
    try { & ([scriptblock]::Create($command)) } catch {
        if ($_.Exception.Message -notlike '*Unexpected installer response*') { throw }
        $rejected = $true
    }
    if (!$rejected) { throw 'HTML response was not rejected' }
    $script:serveHTML = $false
    [IO.File]::WriteAllText((Join-Path $payload 'multica.exe'), 'tampered binary')
    Compress-Archive -Path ($payload + '\*') -DestinationPath $script:fixtureArchive -Force
    $rejected = $false
    try { & ([scriptblock]::Create($command)) } catch {
        if ($_.Exception.Message -notlike '*checksum check*') { throw }
        $rejected = $true
    }
    if (!$rejected) { throw 'Corrupt executable was not rejected' }
    Write-Host 'PASS: copyable command, authenticated downloads, credential handoff, corrupt-package rejection, and HTML rejection.'
} finally {
    $env:USERPROFILE = $savedProfile
}

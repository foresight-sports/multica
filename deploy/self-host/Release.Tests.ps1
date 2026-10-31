$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/Deployment.ps1"
$git = (Get-Command git).Source
$root = Join-Path ([IO.Path]::GetTempPath()) ('multica-release-test-' + [guid]::NewGuid())
$bare = Join-Path $root 'remote.git'
$source = Join-Path $root 'source'
New-Item -ItemType Directory -Path $root | Out-Null
Invoke-DeploymentCommand $git @('init','--bare',$bare) | Out-Null
Invoke-DeploymentCommand $git @('clone',$bare,$source) | Out-Null
Write-DeploymentText (Join-Path $source 'test.txt') 'release one'
Invoke-DeploymentCommand $git @('-C',$source,'add','test.txt') | Out-Null
Invoke-DeploymentCommand $git @('-C',$source,'-c','user.name=Release Test','-c','user.email=release@example.invalid','commit','-m','one') | Out-Null
$first = (Invoke-DeploymentCommand $git @('-C',$source,'rev-parse','HEAD') | Select-Object -Last 1).Trim()
$branch = Get-DeploymentReleaseBranch 'releases' '1.2.3'
New-DeploymentRelease $git $source $branch $first
New-DeploymentRelease $git $source $branch $first
Assert-DeploymentRelease $git $bare $branch $first
Write-DeploymentText (Join-Path $source 'test.txt') 'release two'
Invoke-DeploymentCommand $git @('-C',$source,'add','test.txt') | Out-Null
Invoke-DeploymentCommand $git @('-C',$source,'-c','user.name=Release Test','-c','user.email=release@example.invalid','commit','-m','two') | Out-Null
$second = (Invoke-DeploymentCommand $git @('-C',$source,'rev-parse','HEAD') | Select-Object -Last 1).Trim()
$failed=$false
try { New-DeploymentRelease $git $source $branch $second } catch { $failed=$true }
if (-not $failed) { throw 'Existing release was overwritten' }
Assert-DeploymentRelease $git $bare $branch $first
$failed=$false
try { Assert-DeploymentRelease $git $bare $branch $second } catch { $failed=$true }
if (-not $failed) { throw 'Wrong release SHA accepted' }
foreach ($version in @('', '../bad', '1.2', '01.2.3', '1.2.3 bad')) {
    $failed=$false
    try { Get-DeploymentReleaseBranch 'releases' $version | Out-Null } catch { $failed=$true }
    if (-not $failed) { throw "Invalid release version accepted: $version" }
}
Write-Output "PASS: exact release creation, idempotence, immutable version, SHA verification, invalid versions. Test repository: $root"

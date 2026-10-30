# Isolated end-to-end worker test: real filesystem/state, fake Docker and HTTP.
# It never connects to Docker or the production API.
$ErrorActionPreference = 'Stop'
. "$PSScriptRoot/Deployment.ps1"
$testRoot = Join-Path ([IO.Path]::GetTempPath()) ('multica-deploy-test-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $testRoot | Out-Null
$env:MULTICA_DEPLOY_TEST = $testRoot
$fakeDocker = Join-Path $testRoot 'docker.ps1'
@'
$a = $args
$line = $a -join ' '
if ($a[0] -eq 'compose' -and (Test-Path Env:MULTICA_BACKEND_IMAGE)) { throw 'Compose override was not removed from the process environment' }
if ($line -match ' config --format json') {
    if ($env:MULTICA_DEPLOY_WRONG_IMAGE -eq '1') { '{"services":{"backend":{"image":"upstream:latest"}}}' }
    else { '{"services":{"backend":{"image":"multica-approved-backend:test"}}}' }
    exit 0
}
if ($a[0] -eq 'image') { 'sha256:approved'; exit 0 }
if ($a[0] -eq 'inspect') { 'sha256:approved'; exit 0 }
if ($a[0] -eq 'tag') { exit 0 }
if ($a[0] -eq 'cp') { [IO.File]::WriteAllText($a[-1], 'fake database archive'); exit 0 }
if ($a[0] -eq 'exec') {
    if ($env:MULTICA_DEPLOY_FAIL_BACKUP -eq '1') { exit 3 }
    exit 0
}
if ($line -match ' ps -q ') { 'test-container'; exit 0 }
if ($line -match ' up -d ') { [IO.File]::WriteAllText((Join-Path $env:MULTICA_DEPLOY_TEST 'deployed'), 'yes'); exit 0 }
throw "Unexpected fake Docker call: $line"
'@ | Set-Content -LiteralPath $fakeDocker
$cfg = @{host=$env:COMPUTERNAME;owner_sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value;state_root=(Join-Path $testRoot 'runs');workspace_id='test-workspace';approver_ids=@('owner');docker=$fakeDocker;compose_project='fake';compose_directory=$testRoot;compose_file=(Join-Path $testRoot 'compose.json');env_file=(Join-Path $testRoot 'production.env');postgres_container='fake-db';server_url='https://invalid.test';login_file=(Join-Path $testRoot 'login.json');access_file=(Join-Path $testRoot 'access.json');backend_health='https://invalid.test/readyz';frontend_health='https://invalid.test/'}
Write-DeploymentText $cfg.compose_file '{}'
Write-DeploymentText $cfg.env_file "SECRET=unchanged`nMULTICA_BACKEND_IMAGE=original`nMULTICA_BACKEND_IMAGE_TAG=original`n"
Write-DeploymentJson $cfg.login_file @{token='fake'}
Write-DeploymentJson $cfg.access_file @{client_id='fake';client_secret='fake'}
$configPath=Join-Path $testRoot 'config.json'
Write-DeploymentJson $configPath $cfg
function Invoke-RestMethod {
    param($Uri,$Headers,$MaximumRedirection,$TimeoutSec)
    if ($Uri -like '*/comments') { return @($global:multicaDeployTestApproval) }
    return [pscustomobject]@{workspace_id='test-workspace'}
}
function Invoke-WebRequest { param($Uri,$TimeoutSec,$MaximumRedirection); return [pscustomobject]@{StatusCode=200} }
function New-TestPlan {
    $script:id=[guid]::NewGuid().ToString()
    $script:dir=Join-Path $cfg.state_root $id
    New-Item -ItemType Directory -Path $dir -Force | Out-Null
    $plan=@{deployment=$id;host=$cfg.host;workspace_id=$cfg.workspace_id;issue='test-issue';commit=('a'*40);prepared_at=[DateTime]::UtcNow.AddMinutes(-1).ToString('o');images=@(@{service='backend';tag='multica-approved-backend:test';id='sha256:approved'});config_hash=(Get-FileHash $configPath).Hash;compose_hash=(Get-FileHash $cfg.compose_file).Hash;env_hash=(Get-FileHash $cfg.env_file).Hash;runner_hash=(Get-FileHash "$PSScriptRoot/Deploy.ps1").Hash;library_hash=(Get-FileHash "$PSScriptRoot/Deployment.ps1").Hash}
    Write-DeploymentJson (Join-Path $dir 'plan.json') $plan
    Write-DeploymentJson (Join-Path $dir 'status.json') @{state='awaiting_approval'}
    $digest=(Get-FileHash (Join-Path $dir 'plan.json')).Hash.ToLowerInvariant()
    $global:multicaDeployTestApproval=[pscustomobject]@{id='approval';author_type='member';author_id='owner';issue_id='test-issue';content="Approve deployment $id $digest";created_at=[DateTime]::UtcNow.ToString('o')}
}
try {
    New-TestPlan
    $global:multicaDeployTestApproval.author_type='agent'
    $failed=$false
    try { & "$PSScriptRoot/Deploy.ps1" -Action Run -Config $configPath -Deployment $id } catch { $failed=$true }
    if (-not $failed -or (Test-Path (Join-Path $testRoot 'deployed'))) { throw 'Agent approval changed production' }
    $global:multicaDeployTestApproval.author_type='member'
    $env:MULTICA_DEPLOY_FAIL_BACKUP='1'
    $failed=$false
    try { & "$PSScriptRoot/Deploy.ps1" -Action Run -Config $configPath -Deployment $id } catch { $failed=$true }
    if (-not $failed -or (Test-Path (Join-Path $testRoot 'deployed'))) { throw 'Failed backup did not block deployment' }
    $env:MULTICA_DEPLOY_FAIL_BACKUP='0'
    New-TestPlan
    $env:MULTICA_DEPLOY_WRONG_IMAGE='1'
    $failed=$false
    try { & "$PSScriptRoot/Deploy.ps1" -Action Run -Config $configPath -Deployment $id } catch { $failed=$true }
    if (-not $failed -or (Test-Path (Join-Path $testRoot 'deployed'))) { throw 'Wrong Compose image did not block deployment' }
    Remove-Item Env:MULTICA_DEPLOY_WRONG_IMAGE
    New-TestPlan
    $env:MULTICA_BACKEND_IMAGE=''
    & "$PSScriptRoot/Deploy.ps1" -Action Run -Config $configPath -Deployment $id
    $state=Get-Content (Join-Path $dir 'status.json') -Raw | ConvertFrom-Json
    if ($state.state -ne 'succeeded') { throw "Unexpected final state: $($state.state)" }
    $envText=Get-Content $cfg.env_file -Raw
    if (-not $envText.Contains('SECRET=unchanged') -or -not $envText.Contains('MULTICA_BACKEND_IMAGE_TAG=test')) { throw 'Environment update incorrect' }
    if (-not (Test-Path (Join-Path $dir 'database.dump')) -or -not (Test-Path (Join-Path $dir 'previous-images.json'))) { throw 'Recovery artifacts missing' }
    try { & "$PSScriptRoot/Deploy.ps1" -Action Run -Config $configPath -Deployment $id } catch { }
    $state=Get-Content (Join-Path $dir 'status.json') -Raw | ConvertFrom-Json
    if ($state.state -ne 'succeeded') { throw 'Duplicate invocation overwrote terminal state' }
    Write-Output 'PASS: worker rejects agent approval, blocks on backup failure, deploys approved images, saves recovery artifacts, verifies health, and prevents replay.'
} finally {
    Remove-Item Env:MULTICA_DEPLOY_TEST,Env:MULTICA_DEPLOY_FAIL_BACKUP,Env:MULTICA_DEPLOY_WRONG_IMAGE,Env:MULTICA_BACKEND_IMAGE -ErrorAction SilentlyContinue
    Write-Output "Isolated test artifacts: $testRoot"
}

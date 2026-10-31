#requires -Version 7.0
[CmdletBinding()]
param(
    [ValidateSet('Prepare','Start','Run','Status','Check')][string]$Action = 'Check',
    [Parameter(Mandatory)][string]$Config,
    [string]$Commit,
    [string]$ReleaseVersion,
    [string]$Issue,
    [string]$Deployment,
    [ValidateSet('backend','frontend','both')][string]$Component = 'both'
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. "$PSScriptRoot/Deployment.ps1"
$cfg = Get-Content -LiteralPath $Config -Raw | ConvertFrom-Json
Assert-DeploymentHost $cfg
$root = [IO.Path]::GetFullPath($cfg.state_root)
New-Item -ItemType Directory -Path $root -Force | Out-Null
$configPath = (Resolve-Path -LiteralPath $Config).Path
$scriptPath = $PSCommandPath

function Api([string]$path) {
    $login = Get-Content -LiteralPath $cfg.login_file -Raw | ConvertFrom-Json
    $access = Get-Content -LiteralPath $cfg.access_file -Raw | ConvertFrom-Json
    $headers = @{Authorization="Bearer $($login.token)"; 'CF-Access-Client-Id'=$access.client_id; 'CF-Access-Client-Secret'=$access.client_secret; 'X-Workspace-ID'=$cfg.workspace_id}
    Invoke-RestMethod -Uri ($cfg.server_url.TrimEnd('/') + $path) -Headers $headers -MaximumRedirection 0 -TimeoutSec 30
}
function Docker([string[]]$Arguments) { Invoke-DeploymentCommand $cfg.docker $Arguments }
function Compose([string[]]$Arguments) {
    # Shell variables outrank --env-file. Use only the reviewed production file.
    foreach ($key in @('MULTICA_BACKEND_IMAGE','MULTICA_BACKEND_IMAGE_TAG','MULTICA_WEB_IMAGE','MULTICA_WEB_IMAGE_TAG','MULTICA_IMAGE_TAG','COMPOSE_FILE','COMPOSE_PROFILES')) {
        Remove-Item -LiteralPath "Env:$key" -ErrorAction SilentlyContinue
    }
    Docker (@('compose','--project-name',$cfg.compose_project,'--project-directory',$cfg.compose_directory,'--env-file',$cfg.env_file,'-f',$cfg.compose_file) + $Arguments)
}
function ImageID([string]$name) { (Docker @('image','inspect','--format','{{.Id}}',$name) | Select-Object -Last 1).Trim() }
function Save-State([string]$state, [string]$message = '') {
    Write-DeploymentJson (Join-Path $dir 'status.json') @{state=$state;message=$message;updated_at=[DateTime]::UtcNow.ToString('o');deployment=$Deployment}
}
function Approved($plan) {
    $issueRow = Api "/api/issues/$($plan.issue)"
    if ($issueRow.workspace_id -ne $cfg.workspace_id) { throw 'Deployment issue is outside the configured workspace.' }
    # Invoke-RestMethod can emit a JSON array as one pipeline object. Flatten
    # that response before comparing each comment's scalar author and content.
    $comments = @(Api "/api/issues/$($plan.issue)/comments" | ForEach-Object { $_ })
    Find-DeploymentApproval $plan $digest $comments $cfg.approver_ids
}
function Check-Plan($plan) {
    if ($plan.host -ne $cfg.host -or $plan.workspace_id -ne $cfg.workspace_id -or $plan.deployment -ne $Deployment) { throw 'Deployment target mismatch.' }
    if ((Get-FileHash -LiteralPath $configPath).Hash -ne $plan.config_hash -or (Get-FileHash -LiteralPath $cfg.compose_file).Hash -ne $plan.compose_hash) { throw 'Deployment configuration changed; prepare a new plan.' }
    if ((Get-FileHash -LiteralPath $scriptPath).Hash -ne $plan.runner_hash -or (Get-FileHash -LiteralPath "$PSScriptRoot/Deployment.ps1").Hash -ne $plan.library_hash) { throw 'Deployment runner changed; prepare a new plan.' }
    if ((Get-FileHash -LiteralPath $cfg.env_file).Hash -ne $plan.env_hash) { throw 'Production environment changed; prepare a new plan.' }
    if (([DateTimeOffset]::UtcNow - [DateTimeOffset]$plan.prepared_at).TotalHours -gt 24) { throw 'Deployment plan expired; prepare again.' }
    Assert-DeploymentRelease $cfg.git $plan.repository $plan.release_branch $plan.commit
    foreach ($image in $plan.images) { if ((ImageID $image.tag) -ne $image.id) { throw 'Prepared image changed; prepare a new plan.' } }
}
function Wait-Healthy($plan) {
    for ($attempt=0; $attempt -lt 60; $attempt++) {
        try {
            $ready = Invoke-WebRequest -Uri $cfg.backend_health -TimeoutSec 5 -MaximumRedirection 0
            $web = Invoke-WebRequest -Uri $cfg.frontend_health -TimeoutSec 5 -MaximumRedirection 0
            if ($ready.StatusCode -eq 200 -and $web.StatusCode -eq 200) {
                foreach ($image in $plan.images) {
                    $container = (Compose @('ps','-q',$image.service) | Select-Object -Last 1).Trim()
                    $actual = (Docker @('inspect','--format','{{.Image}}',$container) | Select-Object -Last 1).Trim()
                    if ($actual -ne $image.id) { throw 'Running image does not match approved image.' }
                }
                return
            }
        } catch { }
        Start-Sleep -Seconds 2
    }
    throw 'Deployment health verification failed.'
}

if ($Action -eq 'Check') {
    Docker @('version','--format','{{.Server.Version}}')
    Compose @('ps','--services','--status','running')
    Get-ScheduledTask -ErrorAction Stop | Out-Null
    Write-Output "Deployment host verified: $($cfg.host). Approval required."
    exit
}
if ($Action -eq 'Prepare') {
    if ($Commit -cnotmatch '^[a-f0-9]{40}$' -or $Issue -notmatch '^[a-f0-9-]{36}$') { throw 'Provide an exact 40-character commit SHA and issue UUID.' }
    $releaseBranch = Get-DeploymentReleaseBranch $cfg.release_branch_prefix $ReleaseVersion
    if ($releaseBranch -eq $cfg.branch) { throw 'Release and development branches must differ.' }
    $issueRow = Api "/api/issues/$Issue"
    if ($issueRow.workspace_id -ne $cfg.workspace_id) { throw 'Issue belongs to another workspace.' }
    $Deployment = [guid]::NewGuid().ToString()
    $dir = Join-Path $root $Deployment
    New-Item -ItemType Directory -Path $dir | Out-Null
    Save-State 'preparing'
    try {
        # No user checkout or production checkout is reset or edited.
        $source = Join-Path $dir 'source'
        Invoke-DeploymentCommand $cfg.git @('clone','--no-checkout','--single-branch','--branch',$cfg.branch,$cfg.repository,$source) | Out-Null
        Invoke-DeploymentCommand $cfg.git @('-C',$source,'merge-base','--is-ancestor',$Commit,"origin/$($cfg.branch)") | Out-Null
        Invoke-DeploymentCommand $cfg.git @('-C',$source,'checkout','--detach',$Commit) | Out-Null
        $services = if ($Component -eq 'both') { @('backend','frontend') } else { @($Component) }
        $images = @()
        foreach ($service in $services) {
            $tag = "multica-approved-${service}:$Deployment"
            $dockerfile = if ($service -eq 'backend') { 'Dockerfile' } else { 'Dockerfile.web' }
            $buildArgs = if ($service -eq 'backend') { @('--build-arg',"VERSION=$ReleaseVersion",'--build-arg',"COMMIT=$Commit") } else { @('--build-arg',"NEXT_PUBLIC_APP_VERSION=$ReleaseVersion") }
            Docker (@('build','--label',"org.opencontainers.image.revision=$Commit",'-f',(Join-Path $source $dockerfile),'-t',$tag) + $buildArgs + @($source)) | Out-File (Join-Path $dir "$service-build.log")
            $images += @{service=$service;tag=$tag;id=(ImageID $tag)}
        }
        New-DeploymentRelease $cfg.git $source $releaseBranch $Commit
        $migrations = @(Get-ChildItem -LiteralPath (Join-Path $source 'server/migrations') -Filter '*.up.sql' | Sort-Object Name | ForEach-Object { @{file=$_.Name;sha256=(Get-FileHash -LiteralPath $_.FullName).Hash} })
        $plan = [ordered]@{deployment=$Deployment;host=$cfg.host;workspace_id=$cfg.workspace_id;issue=$Issue;commit=$Commit;branch=$cfg.branch;release_version=$ReleaseVersion;release_branch=$releaseBranch;repository=$cfg.repository;prepared_at=[DateTime]::UtcNow.ToString('o');images=$images;migrations=$migrations;config_hash=(Get-FileHash -LiteralPath $configPath).Hash;runner_hash=(Get-FileHash -LiteralPath $scriptPath).Hash;library_hash=(Get-FileHash -LiteralPath "$PSScriptRoot/Deployment.ps1").Hash;compose_hash=(Get-FileHash -LiteralPath $cfg.compose_file).Hash;env_hash=(Get-FileHash -LiteralPath $cfg.env_file).Hash;rollback='Previous images and database backup retained. Database rollback is manual; image rollback requires review of migration compatibility.'}
        Write-DeploymentJson (Join-Path $dir 'plan.json') $plan
        Save-State 'awaiting_approval'
        $digest = (Get-FileHash -LiteralPath (Join-Path $dir 'plan.json')).Hash.ToLowerInvariant()
        $request = @{deployment=$Deployment;issue=$Issue;workspace_id=$cfg.workspace_id;commit=$Commit;plan_sha256=$digest;host=$cfg.host;prepared_at=$plan.prepared_at;approver_ids=@($cfg.approver_ids)}
        $card = '```multica-deployment-approval' + "`n" + ($request | ConvertTo-Json -Depth 4) + "`n" + '```'
        Write-DeploymentText (Join-Path $dir 'approval-request.md') $card
        Write-Output 'Post approval-request.md verbatim in YOUR agent-authored issue comment, alongside plan and validation details. The approval button replies to that comment to wake you.'
        Write-Output "Prepared $Deployment release $ReleaseVersion ($releaseBranch) at commit $Commit. Review plan.json and build logs."
        Write-Output "Approval comment required on $Issue from a configured human approver:"
        Write-Output "Approve deployment $Deployment $digest"
    } catch { Save-State 'prepare_failed' $_.Exception.Message; throw }
    exit
}
if ($Deployment -notmatch '^[a-f0-9-]{36}$') { throw 'Invalid deployment ID.' }
$dir = Join-Path $root $Deployment
$plan = Get-Content -LiteralPath (Join-Path $dir 'plan.json') -Raw | ConvertFrom-Json
$digest = (Get-FileHash -LiteralPath (Join-Path $dir 'plan.json')).Hash.ToLowerInvariant()
if ($Action -eq 'Status') {
    Get-Content -LiteralPath (Join-Path $dir 'status.json') -Raw
    exit
}
Check-Plan $plan
$approval = Approved $plan
if (-not $approval) { throw 'No matching approval from an authorized human. Production was not changed.' }
if ($Action -eq 'Start') {
    $state = Get-Content -LiteralPath (Join-Path $dir 'status.json') -Raw | ConvertFrom-Json
    if ($state.state -ne 'awaiting_approval') { throw "Deployment is already $($state.state); do not replay it." }
    # Task Scheduler escapes the agent's Windows Job Object and survives its
    # process tree ending as well as a backend/frontend container restart.
    $arguments = '-NoProfile -NonInteractive -File "{0}" -Action Run -Config "{1}" -Deployment {2}' -f $scriptPath,$configPath,$Deployment
    $actionDef = New-ScheduledTaskAction -Execute $cfg.powershell -Argument $arguments
    $principal = New-ScheduledTaskPrincipal -UserId ([Security.Principal.WindowsIdentity]::GetCurrent().Name) -LogonType Interactive -RunLevel Limited
    $settings = New-ScheduledTaskSettingsSet -ExecutionTimeLimit (New-TimeSpan -Hours 1) -MultipleInstances IgnoreNew -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries
    $name = "Multica-Deploy-$Deployment"
    Register-ScheduledTask -TaskName $name -Action $actionDef -Principal $principal -Settings $settings | Out-Null
    Start-ScheduledTask -TaskName $name
    Write-Output "Deployment $Deployment started independently. Use Status to check it."
    exit
}
# An exclusive host-wide lock serializes deployments and makes duplicate starts
# harmless. Approval is checked again inside the worker, immediately before work.
$lock = [IO.File]::Open((Join-Path $root 'deployment.lock'),'OpenOrCreate','ReadWrite','None')
$workerStarted = $false
try {
    $state = Get-Content -LiteralPath (Join-Path $dir 'status.json') -Raw | ConvertFrom-Json
    if ($state.state -ne 'awaiting_approval') { throw 'Deployment already started; inspect its status.' }
    Check-Plan $plan
    $approval = Approved $plan
    if (-not $approval) { throw 'Approval was removed before deployment.' }
    $workerStarted = $true
    Start-Transcript -Path (Join-Path $dir 'worker.log') -Append | Out-Null
    Save-State 'backing_up'
    Write-DeploymentJson (Join-Path $dir 'approval.json') @{id=$approval.id;author_id=$approval.author_id;plan_sha256=$digest}
    $previous = @()
    foreach ($image in $plan.images) {
        $container = (Compose @('ps','-q',$image.service) | Select-Object -Last 1).Trim()
        if (-not $container) { throw 'Expected production service is missing.' }
        $oldID = (Docker @('inspect','--format','{{.Image}}',$container) | Select-Object -Last 1).Trim()
        $oldTag = "multica-rollback-$($image.service):$Deployment"
        Docker @('tag',$oldID,$oldTag) | Out-Null
        $previous += @{service=$image.service;id=$oldID;tag=$oldTag}
    }
    Write-DeploymentJson (Join-Path $dir 'previous-images.json') $previous
    Copy-Item -LiteralPath $cfg.env_file -Destination (Join-Path $dir 'previous.env')
    $dump = "/tmp/multica-$Deployment.dump"
    # Expand DB variables only inside the trusted container, never in logs.
    Docker @('exec',$cfg.postgres_container,'sh','-c',('exec pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc -f '+$dump)) | Out-Null
    Docker @('cp',"$($cfg.postgres_container):$dump",(Join-Path $dir 'database.dump')) | Out-Null
    Docker @('exec',$cfg.postgres_container,'pg_restore','--list',$dump) | Out-Null
    if ((Get-Item -LiteralPath (Join-Path $dir 'database.dump')).Length -eq 0) { throw 'Empty database backup.' }
    Check-Plan $plan
    Save-State 'deploying'
    $envText = Get-Content -LiteralPath $cfg.env_file -Raw
    foreach ($image in $plan.images) {
        $prefix = if ($image.service -eq 'backend') { 'MULTICA_BACKEND' } else { 'MULTICA_WEB' }
        $repo,$tag = $image.tag.Split(':',2)
        $envText = Set-DeploymentEnvValue $envText "${prefix}_IMAGE" $repo
        $envText = Set-DeploymentEnvValue $envText "${prefix}_IMAGE_TAG" $tag
    }
    Write-DeploymentText $cfg.env_file $envText
    # Resolve the actual Compose model before replacing any container. Empty
    # process variables also override --env-file and can select upstream defaults.
    $model = (Compose @('config','--format','json') | Out-String) | ConvertFrom-Json
    foreach ($image in $plan.images) {
        $resolved = $model.services.($image.service).image
        if ($resolved -ne $image.tag -or (ImageID $resolved) -ne $image.id) {
            throw 'Compose image does not match the approved plan; containers were not replaced.'
        }
    }
    Compose (@('up','-d','--no-deps','--pull','never') + @($plan.images | ForEach-Object {$_.service})) | Out-Null
    Save-State 'verifying'
    Wait-Healthy $plan
    Save-State 'succeeded' "Approved commit $($plan.commit) is healthy."
} catch {
    # No automatic database downgrade or code rollback across migrations.
    # Keep the exact old images, env and dump for an explicit recovery decision.
    if ($workerStarted) { Save-State 'failed' ($_.Exception.Message + ' Previous images, environment and database backup are retained where available; inspect worker.log before recovery.') }
    throw
} finally {
    try { Stop-Transcript | Out-Null } catch { }
    $lock.Dispose()
}

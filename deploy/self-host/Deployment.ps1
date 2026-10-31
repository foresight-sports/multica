function Assert-DeploymentHost($Config) {
    if ($env:COMPUTERNAME -ne $Config.host) { throw 'This deployment runner is restricted to its configured host.' }
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    if ($identity -ne $Config.owner_sid) { throw 'Deployment runner must run as its configured Windows owner.' }
}
function Invoke-DeploymentCommand([string]$Executable, [string[]]$Arguments) {
    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Command failed: $([IO.Path]::GetFileName($Executable)) (exit $LASTEXITCODE)." }
}
function Write-DeploymentText([string]$Path, [string]$Text) {
    $temporary = "$Path.$([guid]::NewGuid().ToString('N')).tmp"
    [IO.File]::WriteAllText($temporary,$Text,[Text.UTF8Encoding]::new($false))
    Move-Item -LiteralPath $temporary -Destination $Path -Force
}
function Write-DeploymentJson([string]$Path, $Value) {
    Write-DeploymentText $Path ($Value | ConvertTo-Json -Depth 12)
}
function Find-DeploymentApproval($Plan, [string]$Digest, [object[]]$Comments, [string[]]$Approvers) {
    $phrase = "Approve deployment $($Plan.deployment) $Digest"
    foreach ($comment in $Comments) {
        if ($comment.author_type -eq 'member' -and $comment.author_id -in $Approvers -and
            $comment.issue_id -eq $Plan.issue -and $comment.content.Trim() -ceq $phrase -and
            [DateTimeOffset]$comment.created_at -ge [DateTimeOffset]$Plan.prepared_at) { return $comment }
    }
    return $null
}
function Set-DeploymentEnvValue([string]$Text, [string]$Key, [string]$Value) {
    if ($Key -notmatch '^MULTICA_(BACKEND|WEB)_IMAGE(_TAG)?$' -or $Value -notmatch '^[a-zA-Z0-9._/-]+$') { throw 'Invalid deployment image setting.' }
    $pattern = '(?m)^' + [regex]::Escape($Key) + '=.*$'
    if ([regex]::IsMatch($Text,$pattern)) { return [regex]::Replace($Text,$pattern,"$Key=$Value") }
    return $Text.TrimEnd() + "`n$Key=$Value`n"
}

function Get-DeploymentReleaseBranch([string]$Prefix, [string]$Version) {
    if ($Version -cnotmatch '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z][0-9A-Za-z.-]*)?$') { throw 'Provide an explicit release version, for example 1.2.3 or 1.2.3-rc.1.' }
    if ([string]::IsNullOrWhiteSpace($Prefix)) { throw 'Configure release_branch_prefix on the deployment host.' }
    $branch = $Prefix.TrimEnd('/') + '/' + $Version
    if ($branch -notmatch '^[A-Za-z0-9][A-Za-z0-9._/-]*$' -or $branch.Contains('..') -or $branch.Contains('//') -or $branch.EndsWith('.') -or $branch.EndsWith('.lock') -or $branch.Contains('/.')) { throw 'Invalid release branch prefix.' }
    return $branch
}
function Assert-DeploymentRelease([string]$Git, [string]$Repository, [string]$Branch, [string]$Commit) {
    $ref = "refs/heads/$Branch"
    $rows = @(Invoke-DeploymentCommand $Git @('ls-remote','--heads',$Repository,$ref))
    if ($rows.Count -ne 1 -or $rows[0] -cne "$Commit`t$ref") { throw 'Release branch is missing or no longer points to the prepared commit. Prepare a new plan.' }
}
function New-DeploymentRelease([string]$Git, [string]$Source, [string]$Branch, [string]$Commit) {
    Invoke-DeploymentCommand $Git @('check-ref-format',"refs/heads/$Branch") | Out-Null
    $ref = "refs/heads/$Branch"
    $rows = @(Invoke-DeploymentCommand $Git @('-C',$Source,'ls-remote','--heads','origin',$ref))
    if ($rows.Count -gt 0) {
        if ($rows.Count -eq 1 -and $rows[0] -ceq "$Commit`t$ref") { return }
        throw 'Release version already belongs to another commit. Choose a new version; existing releases are immutable.'
    }
    # An empty expected lease is atomic create-only, never permission to
    # rewrite an existing branch, including a concurrent release preparation.
    Invoke-DeploymentCommand $Git @('-C',$Source,'push',"--force-with-lease=${ref}:",'origin',"${Commit}:$ref") | Out-Null
    $remote = (Invoke-DeploymentCommand $Git @('-C',$Source,'remote','get-url','origin') | Select-Object -Last 1).Trim()
    Assert-DeploymentRelease $Git $remote $Branch $Commit
}

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

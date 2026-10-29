# Run with pwsh -NoProfile -File deploy/self-host/Deployment.Tests.ps1
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
. "$PSScriptRoot/Deployment.ps1"
$plan = [pscustomobject]@{deployment='abc';issue='issue';prepared_at='2026-10-29T00:00:00Z'}
$comment = [pscustomobject]@{id='comment';author_type='member';author_id='owner';issue_id='issue';content='Approve deployment abc digest';created_at='2026-10-29T00:01:00Z'}
function Assert($Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
Assert ($null -ne (Find-DeploymentApproval $plan 'digest' @($comment) @('owner'))) 'Exact human approval rejected'
foreach ($case in @(
    @{field='author_type';value='agent'}, @{field='author_id';value='someone-else'},
    @{field='issue_id';value='another-issue'}, @{field='content';value='Approve deployment abc changed'},
    @{field='content';value='Please quote: Approve deployment abc digest'},
    @{field='created_at';value='2026-10-28T00:00:00Z'}
)) {
    $copy = $comment | ConvertTo-Json | ConvertFrom-Json
    $copy.($case.field) = $case.value
    Assert ($null -eq (Find-DeploymentApproval $plan 'digest' @($copy) @('owner'))) "Accepted invalid approval: $($case.field)"
}
Assert ($null -eq (Find-DeploymentApproval $plan 'digest' @() @('owner'))) 'Missing approval accepted'
$original = "SECRET=keep-this`r`nMULTICA_BACKEND_IMAGE=old`r`nMULTICA_BACKEND_IMAGE_TAG=old-tag`r`n"
$updated = Set-DeploymentEnvValue $original 'MULTICA_BACKEND_IMAGE_TAG' 'approved-id'
Assert ($updated.Contains('SECRET=keep-this')) 'Unrelated environment changed'
Assert ($updated.Contains('MULTICA_BACKEND_IMAGE=old')) 'Wrong component changed'
Assert ($updated.Contains('MULTICA_BACKEND_IMAGE_TAG=approved-id')) 'Tag not replaced'
$rejected = $false
try { Set-DeploymentEnvValue $original 'JWT_SECRET' 'value' | Out-Null } catch { $rejected = $true }
Assert $rejected 'Unexpected environment key accepted'
$rejected = $false
try { Assert-DeploymentHost ([pscustomobject]@{host='definitely-not-this-host';owner_sid='invalid'}) } catch { $rejected = $true }
Assert $rejected 'Wrong host accepted'
foreach ($file in @('Deploy.ps1','Deployment.ps1')) {
    $tokens=$null; $errors=$null
    [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot $file),[ref]$tokens,[ref]$errors) | Out-Null
    Assert ($errors.Count -eq 0) "Parse errors in $file"
}
Write-Output 'PASS: approval identity, exact digest, issue binding, expiry ordering, missing approval, environment isolation, host rejection, script syntax.'

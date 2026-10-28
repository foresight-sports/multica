param([ValidateSet('claude','codex')][string]$Provider,[string]$Executable)
$ErrorActionPreference = 'Stop'
try {
    Write-Host "Multica: $Provider authentication on $env:COMPUTERNAME"
    if ($Provider -eq 'codex') {
        # Use the owner's shared login location, never a task credential copy.
        & $Executable login
        if ($LASTEXITCODE -ne 0) { throw 'Codex login failed.' }
        & $Executable login status
        if ($LASTEXITCODE -ne 0) { throw 'Codex did not confirm a saved login.' }
    } else {
        foreach ($name in @('ANTHROPIC_API_KEY','ANTHROPIC_AUTH_TOKEN','ANTHROPIC_BASE_URL','ANTHROPIC_PROFILE','CLAUDE_CODE_USE_BEDROCK','CLAUDE_CODE_USE_VERTEX','CLAUDE_CODE_USE_FOUNDRY')) {
            if ([Environment]::GetEnvironmentVariable($name)) { throw "Resolve conflicting setting $name before setting up subscription authentication." }
        }
        # Updating is best-effort: registry access is independent of OAuth login.
        try {
            & $Executable update
            if ($LASTEXITCODE -ne 0) {
                Write-Warning 'Claude could not update. Continuing sign-in with the installed version; update Claude when registry access is restored.'
            }
        } catch {
            Write-Warning 'Claude could not update. Continuing sign-in with the installed version; update Claude when registry access is restored.'
        }
        & $Executable setup-token
        if ($LASTEXITCODE -ne 0) { throw 'Claude authorization did not complete.' }
        Write-Host 'Copy the token above and paste it below. Do not send it to Multica or chat.'
        $secret = Read-Host 'Claude token (hidden input)' -AsSecureString
        $pointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($secret)
        try {
            $token = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($pointer).Trim()
            if ($token -notmatch '^sk-ant-oat\S+$') { throw 'Invalid Claude token format; nothing saved.' }
            [Environment]::SetEnvironmentVariable('CLAUDE_CODE_OAUTH_TOKEN', $token, 'User')
        } finally {
            [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($pointer)
            $token = $null
            $secret.Dispose()
        }
        Write-Host 'Saved for future Multica Claude runs. Subscription usage reporting is unavailable in this mode.'
    }
    Write-Host 'Setup completed. Return to Multica. A model request is still needed to verify service access.'
    exit 0
} catch {
    Write-Host 'Setup failed. No diagnostic output or credentials will be sent to Multica.'
    Write-Host $_.Exception.Message
    Start-Sleep -Seconds 10
    exit 1
}

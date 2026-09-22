$ErrorActionPreference = 'Stop'
$directory = Join-Path $env:USERPROFILE '.multica'
$path = Join-Path $directory 'cloudflare-access.json'
New-Item -ItemType Directory -Force -Path $directory | Out-Null
$clientId = Read-Host 'Cloudflare service token Client ID'
$secureSecret = Read-Host 'Cloudflare service token Client Secret' -AsSecureString
$secret = [System.Net.NetworkCredential]::new('', $secureSecret).Password
if ([string]::IsNullOrWhiteSpace($clientId) -or [string]::IsNullOrWhiteSpace($secret)) {
    throw 'Both Cloudflare credentials are required.'
}
# Restrict the file before writing credentials. The runtime runs as this user.
if (!(Test-Path -LiteralPath $path)) { [IO.File]::WriteAllText($path, '') }
$acl = [System.Security.AccessControl.FileSecurity]::new()
$acl.SetOwner([System.Security.Principal.WindowsIdentity]::GetCurrent().User)
$acl.SetAccessRuleProtection($true, $false)
$rule = [System.Security.AccessControl.FileSystemAccessRule]::new(
    [System.Security.Principal.WindowsIdentity]::GetCurrent().User,
    [System.Security.AccessControl.FileSystemRights]::FullControl,
    [System.Security.AccessControl.AccessControlType]::Allow)
$acl.AddAccessRule($rule)
Set-Acl -LiteralPath $path -AclObject $acl
$json = @{ origin = 'https://multica.edgeofglory.dev'; client_id = $clientId.Trim(); client_secret = $secret.Trim() } | ConvertTo-Json
[IO.File]::WriteAllText($path, $json, [System.Text.UTF8Encoding]::new($false))
$secret = $null
$json = $null
Write-Host 'Cloudflare credentials saved for this Windows user. Run multica login.'

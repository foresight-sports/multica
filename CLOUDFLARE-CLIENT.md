# Windows client for Cloudflare Access

This custom client supports the shared Cloudflare service token without changing
Multica user authentication or runtime-registration permissions.

## Cloudflare setup (operator-managed)

Create the shared service token. On the Access application protecting
`multica.edgeofglory.dev`, add a **Service Auth** policy with **Include → Service
Token → the shared token**. Keep the existing company browser-login policy.
The policy must cover the API and `/api/daemon/ws` connection. Do not select a
public Bypass policy. Cloudflare configuration is not changed by the installer.

## Install on each Windows x64 computer

1. Extract the complete `multica-cloudflare-windows-amd64.zip` package.
2. Stop any running Multica CLI/runtime before replacing it.
3. Run `powershell -ExecutionPolicy Bypass -File .\install-cloudflare-client.ps1`.
4. Enter the shared Cloudflare Client ID and Client Secret when prompted. The
   secret is entered without echoing it and is not included in the ZIP.
5. Open a new terminal and run `multica login`, then register through the app's
   Add computer flow using an account permitted to register runtimes.

The installer backs up the old binary, selects the public Multica address, and
disables automatic upstream updates so they cannot replace this custom build.
Use the same Windows account for installation and running the runtime.
The package is a locally built, unsigned Windows x64 binary.

## Credentials and rotation

The configuration script writes `%USERPROFILE%\.multica\cloudflare-access.json`
with permissions limited to the current Windows user. It contains `origin`,
`client_id`, and `client_secret`. The origin is `https://multica.edgeofglory.dev`.
The secret is stored as a file protected by Windows permissions, not encrypted.
Rerun `configure-cloudflare-access.ps1` on each computer when rotating the shared
token. Credentials are read per request, so restarting is not required for rotation.

For a separately managed credential file, set `MULTICA_CLOUDFLARE_ACCESS_FILE`
in the CLI/runtime process environment. An explicitly configured missing file,
malformed file, or incomplete credentials fail with an error containing no secrets.
With no default credential file, ordinary client behavior is unchanged.

Credentials are added only to matching HTTPS requests and WSS handshakes.
HTTP requests, other hosts, other ports, and redirected requests to other origins
do not receive the service token. Multica's Authorization header remains separate.

## Validation

Unit tests cover credential scope, WSS URL handling, redirect isolation, and
invalid configuration. Live Cloudflare login and runtime registration require
the operator's Service Auth policy and credentials and must be verified afterward.

Validation completed: full backend make test with race detection passed; the
Windows x64 binary ran successfully with --version; both PowerShell scripts
passed parsing. The initial focused run had a process-tree cancellation timing
failure; the full suite subsequently passed that daemon package. Live Cloudflare
validation is pending operator configuration.

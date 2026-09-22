# Permission access

## Who manages access

Set a comma-separated list of manager account emails in the deployment `.env`:

```
MULTICA_PERMISSION_MANAGER_EMAILS=manager@example.com,another@example.com
```

Recreate the backend after changing this operator-only list. An empty list gives
nobody management access. Workspace roles do not bypass it. The UI cannot change
this list or grant permission-management access. Listed accounts must sign in
normally; adding an address does not create an account or bypass signup rules.

Managers see **Settings → Permission access** (`?tab=permissions`) in web and
shared desktop settings. The page manages runtime registration across the entire
installation, not just the currently selected workspace.

## Runtime registration

The page accepts one email per line (commas also work). Save explicitly to apply
changes immediately. Emails are validated, normalized, and deduplicated. Empty
lists deny everyone. Users also need membership in the target workspace.
Owners/admins have no exception. A manager can edit this list without having
permission to register runtimes themselves.

Before the first UI save, these existing environment values supply the policy:

```
MULTICA_RUNTIME_REGISTRATION_RESTRICTED=true
MULTICA_RUNTIME_REGISTRATION_ALLOWED_EMAILS=test@foresightsports.com
```

After the first save, the database row is authoritative and survives restarts.
Changing the bootstrap values no longer overrides saved rules. Each save records
the editor, time, and revision. Conflicting edits return 409 and require reloading;
failed saves retain the draft. A database failure denies registration.

The registration endpoint checks every attempt, including custom profiles and
re-registration. Use an allowed user's login/PAT; machine-only daemon tokens
cannot enroll while restricted. Existing runtime heartbeats and execution are
unchanged. This does not revoke existing runtimes or change custom-profile editing
permissions. Managed Cloud provisioning is a separate, unconfigured service.

User responses expose only computed booleans, not email lists. The Runtimes page
hides Add computer without a grant. Reload existing tabs to refresh account
permissions after another manager changes access.

## Build and deployment

Both custom images must be preserved during upgrades:

```
MULTICA_BACKEND_IMAGE=multica-backend-foresight
MULTICA_BACKEND_IMAGE_TAG=permissions-v3
MULTICA_WEB_IMAGE=multica-web-foresight
MULTICA_WEB_IMAGE_TAG=permissions-v3
```

```
docker build -t multica-backend-foresight:permissions-v3 --build-arg VERSION=0.4.44-foresight.3 .
docker build -f Dockerfile.web -t multica-web-foresight:permissions-v3 --build-arg NEXT_PUBLIC_APP_VERSION=0.4.44-foresight.3 .
docker compose -f docker-compose.selfhost.yml -f docker-compose.tunnel.yml --profile tunnel up -d --no-deps backend frontend
```

The backend applies migrations 491/492 (policy table and concurrent unique index).
No existing application data is rewritten. Rebase the custom code and rebuild
both images for future upgrades. Upstream images do not enforce these rules.
Rolling back to permissions-v2 reverts to the environment-based runtime policy;
it leaves the stored policy intact but does not enforce edits made through this page.

## Verification

Backend checks use an isolated database through
`docker compose -f docker-compose.permission-test.yml run --rm tests` (`make test`
with race detection). Run sqlc generation with `ENV_FILE=env.permission-test`, not
the production `.env`. Frontend checks use Dockerfile.permission-web-test after
building Dockerfile.web's deps target as multica-web-permission-deps; mount
apps/desktop/src and scripts read-only at the matching /app paths for web fixtures.

The shared workspace permission gate and action-keyed policy table can be extended
for future restrictions. Add a named policy, server enforcement, computed user
capability, and UI editor together; unknown actions are denied.

## Verification status (2026-09-22)

- Full backend `make test` passed with race detection.
- Frontend type checking and linting passed (existing warnings only).
- All 7,322 frontend tests passed: 266 web, 1,894 core, 5,162 shared views.
- The final draft-preservation adjustment passed its focused API/UI tests and
  type/lint checks afterward.
- Both permissions-v3 production images built successfully.
- Deployed both permissions-v3 images with kpeterson@foresightsports.com as the
  environment-configured permission manager. Runtime registration remains limited
  to test@foresightsports.com until a manager saves a policy in Settings.
- Backend health and the proxied login page returned HTTP 200. The policy table
  and valid unique index exist; the Cloudflare connector reports four ready connections.
- Environment and database backups were saved before deployment.

# Operator policy: runtime registration

Set these values in `.env` and recreate the backend:

```
MULTICA_RUNTIME_REGISTRATION_RESTRICTED=true
MULTICA_RUNTIME_REGISTRATION_ALLOWED_EMAILS=test@foresightsports.com
```

The email list is comma-separated and compared case-insensitively against the
persisted account email. The authenticated user must also be a member of the
requested workspace. Owners and admins have no exemption. An empty list with
the restriction enabled denies everyone. With the restriction disabled,
upstream registration behavior is preserved.

This guards POST /api/daemon/register, including repeat registration and custom
profile registration. Enroll daemons using an allowed user's PAT/login. Machine
(mdt_) credentials alone cannot register while restricted. Existing runtime
heartbeats and task execution are unchanged; this policy does not revoke existing
runtimes. Managed Cloud provisioning is a separate service and is not configured
on this installation. Custom profile editing retains upstream role permissions.

The shared `requireWorkspacePermission` helper in
`server/internal/handler/workspace_permission.go` is the extension point for
future action-specific user lists. Unknown permission names deny access. Keep
membership and persisted identity checks for every additional action.

This deployment uses a custom backend image. Upstream images do not contain this
policy. Rebase and rebuild the custom image when upgrading; do not replace it
with `latest`. No database migration is required for this change.

## Deployment and updates

The deployment `.env` pins the backend independently from the frontend:

```
MULTICA_BACKEND_IMAGE=multica-backend-foresight
MULTICA_BACKEND_IMAGE_TAG=permissions-v1
```

Build and deploy from this checkout:

```
docker build -t multica-backend-foresight:permissions-v1 --build-arg VERSION=0.4.44-foresight.1 .
docker compose -f docker-compose.selfhost.yml -f docker-compose.tunnel.yml --profile tunnel up -d --no-deps backend
```

After changing allowed emails in `.env`, rerun the Compose command to apply it.
Changes are operator-managed; no new settings UI is included.

For isolated verification on this Windows host (Go/make are supplied by Docker):

```
docker compose -f docker-compose.permission-test.yml run --rm tests
```

The test Compose project has its own PostgreSQL instance and uses
`env.permission-test` with `make test`. It never connects to the production DB.
Stop its containers afterward with `docker compose -f docker-compose.permission-test.yml down`.

Rollback: set `MULTICA_BACKEND_IMAGE=ghcr.io/multica-ai/multica-backend` and
`MULTICA_BACKEND_IMAGE_TAG=latest`, then recreate the backend. This removes the
custom restriction; preserve the custom image if the policy must remain enforced.

## Validation (2026-09-19)

- Full `make test` passed with race detection in the isolated, non-root Docker environment.
- Registration regression cases cover allowed and denied members/admins/owners,
  an empty allowlist, cross-workspace membership, and forged user headers on
  daemon-token requests. Existing daemon registration tests also passed.
- Deployment confirmed the custom image and both policy environment values.
- Backend health and origin login returned HTTP 200; Cloudflare had four ready connections.
- The allowlisted account was not yet registered; it must sign in and join a workspace.

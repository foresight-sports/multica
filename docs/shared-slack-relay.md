# Shared FS Caddie Slack app

Onyx is the sole Socket Mode receiver. Multica's installation uses explicit
`relay` mode, retaining its encrypted bot credentials for replies, reactions,
directory access and files. Other installations default to `socket_mode`.
Credential reinstall preserves relay mode.

The coordinated runbook is in the Onyx repository at
`docs/slack-multica-relay.md`. Follow it for both deployments. The release owner
selected **0.6.8** for this change; use `-ReleaseVersion 0.6.8` during Prepare.
Do not reuse 0.6.7 or move an existing release ref to a different commit. Version
selection does not authorize deployment or live Slack changes. For Foresight deployment use
[the installed host runner](../deploy/self-host/README.md), exact pushed commits,
immutable release branches and the existing deployment approval procedure.

Server configuration: `MULTICA_SLACK_RELAY_CONFIG_FILE` names a service-only JSON
file containing an array of `{installation_id, workspace_id, app_id, team_id,
secret}` bindings. Use a dedicated random shared secret of at least 32 characters.
Never put it in installation JSON, UI responses, agent task environments or logs.
Retain `MULTICA_SLACK_SECRET_KEY` and the existing Slack credentials.

Self-host Compose deployments can include `docker-compose.slack-relay.yml` after
`docker-compose.selfhost.yml`. The operator sets `MULTICA_SLACK_RELAY_SECRET_FILE`
to the absolute service-owned credential file path. This optional overlay mounts
that file read-only into the backend only and sets its container configuration
path. The file must already exist. Merely setting a host environment variable in
`.env` does not mount a file or pass it through the base self-host Compose file.
The host owner must incorporate this overlay into the installed deployment
runner's approved Compose configuration before preparing the release. Do not
edit the installed runner's policy or production environment from an agent task.


Human workspace-admin APIs:

- `GET /api/workspaces/{workspace}/slack/installations/{installation}/relay-preflight`:
  revalidates Slack app/team/bot and configured channel membership without opening
  a Socket Mode connection or sending a message.
- `PUT /api/workspaces/{workspace}/slack/installations/{installation}/inbound-mode`
  with `{"mode":"relay","expected_mode":"socket_mode"}`: explicit mode change,
  accepted only after the dedicated credential is bound correctly. Restart all
  backend replicas in relay mode as the coordinated cutover barrier.
- `GET /api/workspaces/{workspace}/slack/relay`: metadata-only relay health and
  receipts, also shown in Slack settings. Contact is stale after 90 seconds.
- `POST /api/workspaces/{workspace}/slack/relay/{receipt}/replay`: explicitly
  retries failed/held/waiting-root intake after the original workflow is enabled.
  Delivered receipts cannot be replayed here.

Authenticated server APIs: `GET /api/slack/relay/{installation}/routes` and
`POST /api/slack/relay/{installation}/events`. HMAC binds timestamp, nonce, method,
path and body; nonces are persisted across restarts. Routing reads are diagnostic,
not an authorization cache. The first event receipt pins ownership by
installation/channel/message timestamp, including disabled workflows. A disabled
workflow holds new events until explicit replay. Outages never authorize Onyx
to investigate support messages.

Multica's existing intake worker drains receipts transactionally into support
intake. Root/reply ordering, issue deduplication, coordinator selection, instance
audience wording, FS Caddie identity, confidence/escalation, backlog creation and
outbound delivery reconciliation remain in that pipeline. Investigations never
authorize fixes or PRs. Missing Jira credentials must escalate without borrowing
a human connection. Attachments remain untrusted evidence and download with the
installation's server-side bot token.

Apply migrations 584–590 before routing traffic. New indexes are separate
concurrent migrations. Keep durable queues and deduplication IDs during upgrades
or recovery; do not downgrade their tables while receipts remain.

Initial live verification is restricted to **#fs-caddie-test** after revalidating
all identifiers and agreeing on the cutover. Do not enable
**#foresight-app-bugs-and-support**, alter the live Onyx connection, or send live
messages as part of automated tests. End-to-end success requires one real private
channel message to produce one Multica issue and its intended threaded result,
with no competing Onyx answer.


## Local verification

Use an isolated migrated Postgres database, never the deployment database:

```bash
cd server
DATABASE_URL='<isolated Postgres URL>' go test ./internal/handler ./internal/integrations/slack -run 'TestSlackRelay|TestSlackIntake|TestSlackSupport|TestRelay' -count=1
ONYX_RELAY_TEST_ROOT='<absolute sibling Onyx checkout>' DATABASE_URL='<same isolated Postgres URL>' go test ./internal/handler -run TestSlackRelayOnyxContractOneIssueNoCompetingAnswer -count=1
```

The cross-repository test launches a local TLS HTTP server and calls Onyx's Python
capture/forwarder using temporary Postgres schemas. It checks one issue, one
Multica receipt, two delivered Onyx event variants, and zero Onyx answer calls.
It never contacts Slack or an LLM. The Onyx checkout needs its existing `.venv`.

Frontend checks: `pnpm --filter @multica/views typecheck`, targeted Vitest runs
for `slack-relay-status.test.tsx` and `slack-intake-section.test.tsx`, and the core
`slack-relay-schema.test.ts` test. The relay panel was also inspected in desktop
and mobile browser previews using synthetic metadata.

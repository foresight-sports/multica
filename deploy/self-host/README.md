# Host-only Multica deployments

The Multica Deployer agent is bound to the host runtime with **no portable execution profiles**. The existing `agent_allows_runtime` gate enforces that binding. Do not add profiles: doing so deliberately changes an agent to portable scheduling. An offline host means waiting for that host.

The host installs `Deploy.ps1` and `Deployment.ps1` outside agent worktrees with a local `config.json`. Configuration pins the Windows host and owner SID, Multica workspace, allowed human approver IDs, repository/branch, absolute tool paths, production Compose/env files, database container and health URLs. It contains paths to existing login files, not copied credentials. Keep its state directory owner/SYSTEM-only: backups contain production data and the previous environment.

## Direct operator deployment

An explicit human deployment instruction in Codex authorizes an operator-managed deployment without another issue-comment approval. Record the conversation authorization separately from automated approval receipts. Preserve exact image/configuration validation, an exclusive deployment lock, database and image backups, and post-deployment health/image verification. Never fabricate an approval comment. Autonomous Multica deployments still require the workflow below.

## Workflow

The deployer must post the generated `approval-request.md` block in its own agent-authored comment alongside the plan and validation details. The UI renders **Approve deployment** for listed human approvers. Clicking it posts the exact approval phrase as that signed-in human's reply, waking the authoring deployer through normal comment routing. Requests in descriptions or human-authored comments cannot do this. The card is presentation only; the host still independently validates its configured approvers and immutable plan. A saved approval is not execution confirmation: inspect actual task and runner status before reporting progress.

1. Development agents push and test changes on `foresight-custom/develop`. Delegate deployment to Multica Deployer on a Multica issue.
2. The deployer calls the installed runner with `-Action Prepare -ReleaseVersion <version> -Commit <40-character SHA> -Issue <issue UUID> -Component backend|frontend|both`. Preparation checks the commit belongs to the configured remote branch, clones into an isolated directory, builds images, and writes an immutable approval plan. It does not change production containers. Review tests and migration compatibility before requesting approval; the runner records migration hashes but does not replace application tests or rehearse migrations.
3. The agent posts the commit, components, image IDs, test results, migration impact, rollback limitations and exact approval phrase printed by Prepare. The configured human replies directly to that comment with the phrase, on the same issue, so Multica wakes the authoring deployer. The phrase includes the deployment ID and SHA-256 of the entire plan. Agent comments, another account, another issue, stale comments, and different plans cannot approve a deployment. Plans expire after 24 hours. Deleting the approval before the worker starts prevents execution.
4. The agent calls `-Action Start -Deployment <UUID>`. This revalidates approval and registers a one-shot Windows scheduled task. The worker rechecks approval, host identity, configuration hashes, environment hash and built image IDs. It takes an exclusive deployment lock, retains previous images/environment, creates and validates a PostgreSQL archive, updates only the selected image settings and replaces only selected services.
5. The worker verifies backend readiness, frontend HTTP health and exact running image IDs. It writes persistent status and logs independently of the agent process and application containers. The agent calls `-Action Status -Deployment <UUID>` after reconnection and reports the result. Use the same command if the agent task ends before deployment does; do not replay Start.

Approval is an application workflow control. It is not an OS sandbox against arbitrary code already running as the host owner. The deployer must use only the installed runner and never edit its policy, host credentials, production environment, Docker containers or approval records directly.

## Failure and recovery

Each deployment directory retains `plan.json`, `approval.json`, `status.json`, build logs, `worker.log`, `previous-images.json`, `previous.env` and `database.dump` where those stages completed. `failed` requires inspecting these records. A host reboot can leave an intermediate status; inspect the scheduled task result and containers before recovery. There is no automatic replay after reboot or automatic database downgrade.

Image rollback is a separate approved operation after checking schema compatibility. Previous images are pinned with `multica-rollback-<service>:<deployment ID>` tags. Never restore the database automatically: that could discard writes made after the deployment. Backend startup runs the application's normal migrations.

The scheduled task runs as the existing interactive Windows owner and survives agent/server restarts, but requires that Windows user to remain logged in. It does not upgrade the daemon, deploy other platforms, publish release assets or change instance secrets.

## Verification

Run `pwsh -NoProfile -File deploy/self-host/Deployment.Tests.ps1` for approval and policy tests and `pwsh -NoProfile -File deploy/self-host/Worker.Tests.ps1` for an isolated worker lifecycle test with fake Docker/HTTP. The installed runner's `-Action Check` verifies host identity, Docker, Compose services and Task Scheduler without deploying. Test detached scheduling with a harmless task before enabling the agent. Automated production deployment still requires a prepared plan and real human approval. Run `pwsh -NoProfile -File deploy/self-host/Release.Tests.ps1` to verify immutable release creation against an isolated local Git remote.


## Development and release branches

Use a stable development branch in `config.branch` and in the workspace repository settings. Do not put the current release branch in workspace instructions. Enable direct delivery for Multica in Code delivery settings, targeting that same development branch. Other workspaces default to normal PRs.

Configure `release_branch_prefix` (for example `foresight-custom/releases`). Every Prepare requires `-ReleaseVersion <version>` supplied by the release owner. Preparation builds the exact pushed commit, then creates `<prefix>/<version>` at that commit before producing the approval plan. An existing branch is reusable only at the identical commit. Atomic creation rejects concurrent attempts to reuse a version. The plan binds the version, branch, commit and images; Start and Run recheck the remote release ref. Preparation publishes a release branch but does not replace production containers. Never move a published release branch or guess a new version.


## PR delivery and review configuration

Code delivery settings are under workspace settings, with a workspace default and optional complete project overrides. The default is a normal PR. Configure GitHub AI reviewer logins (including `[bot]` for bot identities), then the human logins/team slugs to request after every configured AI reviewer approves the current head. These identities are not inferred from display names. An AI review represented only as prose saying “approved” does not count as a GitHub approval review.

Connect the repository's GitHub App installation to each workspace. The App needs pull request read/write access for review follow-up and reviewer requests; existing pull-request webhooks drive merge-to-Done. The server reconciles linked open PRs periodically, so missing review webhooks do not lose feedback. It processes submitted reviews and inline review comments by the configured AI accounts, revalidates the live head, and wakes the issue assignee (or squad coordinator) under the original requester's invocation permissions. It does not execute instructions embedded in review text. Durable receipts suppress repeated feedback and requests across restarts. Pending/busy runs and connection failures are retried and shown in Code delivery settings. The existing merge setting defaults to Done and waits for all linked PRs; explicit workspace/per-issue opt-outs remain respected.

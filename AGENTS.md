# Repository Instructions

Multica is a task management platform where people and agents collaborate on issues. These instructions apply to all coding agents working in this repository.

## Scope and Reading Order

- Before changing `apps/mobile/`, also read [apps/mobile/AGENTS.md](apps/mobile/AGENTS.md), even if your tool does not load nested instructions automatically. Platform-specific sections below apply only to the named platform.
- For naming, translations, or Chinese UI/docs copy, read [conventions.mdx](apps/docs/content/docs/developers/conventions.mdx) and [conventions.zh.mdx](apps/docs/content/docs/developers/conventions.zh.mdx).
- Maintain shared rules here and mobile-specific rules in the mobile file. `CLAUDE.md` files only import them. Update instructions in the same change that alters the referenced workflow or boundary; do not add incident timelines, dependency version lists, or duplicate rules.

## Sharing Rules and Package Boundaries

| Location | Responsibility and constraints |
| --- | --- |
| `server/` | Go backend; Chi, sqlc, WebSocket |
| `packages/core/` | Headless logic, API client, Query hooks, shared Zustand stores. No UI libraries, `react-dom`, `localStorage`, or `process.env`; use `StorageAdapter` for persistence. |
| `packages/ui/` | UI primitives and shared styles. No business logic or `@multica/core` imports. |
| `packages/views/` | Shared web/desktop pages and business components. No store definitions, `next/*`, or `react-router-dom`; use `NavigationAdapter`, `useNavigation()`, and `<AppLink>`. |
| `apps/web/` | Next.js routes/layouts and web-only UI. Framework APIs stay here; shared navigation adapters live in `apps/web/platform/`. |
| `apps/desktop/` | Electron and desktop-only UI/state. Application navigation goes through `apps/desktop/src/renderer/src/platform/`. |
| `apps/mobile/` | Independent Expo/React Native client: owns UI, state, hooks, providers, i18n, build, and release. Shares core types and pure utilities, including platform-independent schemas. |
| `apps/docs/` | Fumadocs documentation site |

- Dependency direction is `views -> core + ui`; core and ui remain independent. Shared packages export raw TypeScript compiled by consuming apps.
- Extract logic used by both web and desktop into the appropriate shared package. Keep framework/Electron APIs in the app layer; inject platform-specific UI through props/slots.
- Wire shared features into both web routes and the desktop router or overlay. Reuse existing guards/providers such as `DashboardGuard` in `packages/views/layout/`.
- Each workspace declares its directly imported external dependencies. Use `catalog:` for shared dependencies; mobile pins Expo/React Native dependencies in its own manifest.

## Development and Verification

Use `Makefile`, workspace `package.json` files, and `pnpm-workspace.yaml` for current commands and versions. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup and worktree operations.

- Use the checkout's managed environment: `make up`, `make status`, `make down`. `make down` preserves data; `make destroy` removes the environment and its data.
- Worktrees share PostgreSQL but have isolated databases/ports. Use the environment scripts and `.env.worktree`; do not copy the main checkout's `.env` or manually create a database through an assumed PostgreSQL instance.
- Regenerate sqlc with `make sqlc` after SQL changes.
- Run the narrowest useful checks while iterating, then broaden when risk warrants it. Report what actually ran and any skipped checks.

Run these from the repository root:

| Scope | Checks |
| --- | --- |
| Frontend excluding mobile | `pnpm typecheck`, `pnpm lint`, `pnpm test` |
| Go backend | `make test` |
| End-to-end | `pnpm exec playwright test` |
| Combined web/backend verification | `make check` |
| Mobile | Commands in [apps/mobile/AGENTS.md](apps/mobile/AGENTS.md#verification) |

Root frontend commands and `make check` do not verify mobile. Docs-only changes can use link/reference checks and `git diff --check`; state that code tests were not run.

## State Rules

- Full run logs show machine and model in the visible header. Resolve the machine from the task's runtime ID, and models from reported task usage or its saved execution selection; never substitute the agent's current configuration. Preserve the runtime ID if its registration is unavailable, and show missing historical model data explicitly.

- TanStack Query owns API/server data. Zustand owns client state such as filters, drafts, modals, and tab layout; persist only durable preferences/drafts/layout, not server data or ephemeral UI state.
- Web/desktop shared stores live in `packages/core/`. Desktop platform stores remain in desktop; mobile stores remain in mobile. Do not define stores in `packages/views/`.
- On web/desktop, workspace identity is route-driven; platform mirrors exist only for request headers, storage namespaces, and reconnects. React Context is for platform plumbing, not a second server-state store.
- Among stores, only auth/workspace stores may call `api.*` directly; other server interactions belong in queries/mutations.
- Workspace-scoped query keys include `wsId`; account-level keys remain account-scoped. Hooks needing workspace context accept `wsId` unless guaranteed to run under its provider.
- Zustand selectors return stable references; use shallow comparison for allocated objects/arrays.
- WebSocket events patch or invalidate Query caches, not server payloads in Zustand. Clearing client-owned pointers is allowed with one responder and a self-initiated guard when this client can cause the event.
- Optimistic field patches require a predictable result, rare failure, trivial rollback, and staying on the current screen. Snapshot before patching, roll back on failure, and invalidate uncertain projections on settle.
- Create/delete/leave and confirmation flows await the server before navigation or cleanup; do not optimistically delete entities. Exceptions: the existing workspace-leave race noted under Desktop Rules, and mobile inbox mark-read as documented in its instructions.
- Message sends use visible pending state and retry on failure.

## API Compatibility

Installed desktop clients may talk to newer backends. Preserve response compatibility at the API boundary.

- UI-consumed JSON passes through a zod schema and `parseWithFallback`, not an `as T` cast. Web/desktop use `packages/core/api/schema.ts`; mobile uses its own request helpers.
- Provide defaults for optional fields and fallbacks for unknown server enums. Prefer explicit boolean checks; avoid tying critical affordances to a single backend flag when other contract signals are available.
- When adding/changing an endpoint, update its schema and malformed-response tests.

## Database and Migration Rules

- Do not add foreign keys, cascading deletes, or cascading updates. Validate relationships and clean up dependents in application code, using a transaction when the operation must be atomic.
- Every migration-created index, including indexes on new tables, uses `CREATE [UNIQUE] INDEX CONCURRENTLY`. Each concurrent index build gets its own single-statement migration file; the runner executes files outside an explicit transaction.
- Conditionally skipped migrations are still recorded in `schema_migrations`. Later DDL touching conditional objects must be idempotent (`IF EXISTS` / `IF NOT EXISTS`); document recovery if the missing object would break runtime behavior.

## Backend UUID Rules

In `server/internal/handler/`, distinguish UUID sources before using them in writes:

- UUID-or-human-readable resource params: resolve with loaders such as `loadIssueForUser`, `loadSkillForUser`, `loadAgentForUser`, or `requireDaemonRuntimeAccess`, then write using the resolved `entity.ID`.
- Pure UUID request input: `parseUUIDOrBadRequest(w, s, fieldName)`; return immediately when `ok=false`.
- Trusted sqlc/test-fixture round-trips: `parseUUID(s)`, which panics on invalid input.
- Outside handlers: `util.ParseUUID(s)` and check the error.

Workspace-scoped queries filter by `workspace_id`; membership gates access and `X-Workspace-ID` selects the workspace. Assignees are polymorphic: interpret `assignee_id` together with `assignee_type`.

Instance configuration and instance agent definitions are global. Only configured permission managers may edit global instructions and permission access lists. Instance-agent creation and agent editing have independent account-level policies, in addition to resource membership and ownership checks. Instance agents use workspace-scoped `agent` bindings linked by `instance_agent_id` and `instance_source_agent_id`. Runtime, model, skills, access and execution configuration belong to the source agent and are synchronized transactionally by database triggers. Other workspaces may enable/disable their binding, but cannot edit configuration or reveal its secrets. Preserve workspace opt-outs and separate task histories. Shared-runtime claims require a verified definition/source/binding relationship and keep all task context and task-token access scoped to the target workspace. Never grant a runtime blanket access to another workspace. Global instructions are loaded on every claim; a failed read must not launch a run without them.

Instance agent creation uses the normal manual/AI creation draft and `POST /api/agents` with `instance_agent: true`. Default drafts with instance-agent creation permission to instance scope, preserve explicit opt-outs during draft restoration, and keep users without that permission workspace scoped. Link the complete source setup before provisioning other workspaces, in one transaction under the instance advisory lock.

## Desktop Rules

- Workspace session routes are tab destinations. Pre-workspace one-shot flows (create workspace, accept invite) use `WindowOverlay` in `apps/desktop/src/renderer/src/stores/window-overlay-store.ts`, not new routes. Stale workspace tabs heal by dropping stale tab groups.
- Workspace route layouts own `setCurrentWorkspace(slug, uuid)` from `@multica/core/platform`; leaving workspace context calls `setCurrentWorkspace(null, null)`.
- Cross-workspace navigation uses the adapter's `switchWorkspace(slug, targetPath)` flow; do not bypass it with direct router navigation.
- Workspace delete awaits the server. Existing workspace leave clears/navigates first to avoid the `member:removed` race; this is known debt in `packages/views/settings/components/workspace-tab.tsx`, not a pattern for new flows.
- Full-window views outside the dashboard shell mount `<DragStrip />` from `@multica/views/platform` as the first flex child. Interactive controls in the top 48px need `WebkitAppRegion: "no-drag"`.

## UI Copy

- Descriptions are optional and omitted by default. Do not restate titles, labels, values, statuses, or button actions. Add help only for a non-obvious choice, constraint, consequence, or next step; state each fact once beside the relevant control.
- Keep permissions, cost, destructive consequences, execution prerequisites, and error recovery visible when relevant. Put advanced usage and diagnostics in accessible, explicit help. Preserve labels and accessible names; do not move redundant prose wholesale into `sr-only` text.
- Review copy with its surrounding controls and all supported translations, including mobile's independent copy. Follow the UI copy rules in the existing conventions pages; a description prop is not a requirement to write a paragraph.

## Web/Desktop UI Rules

- For Button and Dialog usage, read `packages/ui/docs/button.md` and `packages/ui/docs/dialog.md`. These component contracts also power UI Lab documentation.

- Prefer existing shadcn/Base UI primitives. Add components with `pnpm ui:add <component>`.
- For `pnpm ui:add @reui/<name>`, decline overwrite prompts. Keep `REUI_LICENSE_KEY` in the environment, never in repo files. Adapt vendored primitives into `packages/ui/components/ui/` and compositions into `packages/views/`.
- Use shared semantic tokens in `packages/ui/styles/`. Typography uses the role-named `--text-*` scale in `packages/ui/styles/tokens.css`, not Tailwind's default size ramp.
- Selected states remain identifiable on hover. Handle overflow, long text, and scrolling deliberately; avoid unnecessary local state and dividers.

## Testing

- Tests live beside their implementation: shared logic in core, shared components in views, platform wiring in apps, E2E in `e2e/`, Go tests in server. Do not test shared behavior in app suites.
- Give each behavior one canonical test layer: helper tests own parsing/state matrices; component tests cover wiring, accessibility, happy paths, and named regressions. Prefer a failing regression test before behavioral fixes.
- DOM-free `.test.ts` files start with `// @vitest-environment node`; do not use it if it would silently switch the code under test to an SSR path.
- Views tests must not mock `next/*` or `react-router-dom`. Mock stores with their Zustand callable shape plus `getState`; mock API calls at `@multica/core/api`.
- E2E setup/teardown uses `TestApiClient`.
- DB-backed Go tests use `server/internal/testutil` fixtures (`dbfx.Issue`, `dbfx.Task`, `dbfx.Insert`) and `testutil.Call(h, req).Want(status).JSON(&out)`. Keep product assertions and case-specific diagnostics in the test, not fixture helpers.
- Default tests must not resolve or execute user-installed agent CLIs; pass test-created fake or missing executable paths. New default agent commands go in `scripts/agent-cli-command-names.txt`.
- Only run real-agent smoke tests when explicitly authorized. Gate them behind `agentintegration` and check `MULTICA_RUN_REAL_AGENT_SMOKE=1` before executable lookup/account access. Run the specific test: `(cd server && MULTICA_RUN_REAL_AGENT_SMOKE=1 go test -tags=agentintegration ./pkg/agent -run '<test-name>' -count=1 -v)`.

## Change and Delivery Rules

- Keep changes scoped; reuse existing patterns. Code comments are English.
- Do not add internal compatibility shims, dual writes, fallback paths, or legacy adapters unless requested. This does not relax API response compatibility above.
- New global pre-workspace routes use a single word or `/{noun}/{verb}`, not hyphenated root names. Update `server/internal/handler/reserved_slugs.json`, run `pnpm generate:reserved-slugs`, and commit `packages/core/paths/reserved-slugs.ts` when changing reserved slugs.
- Use atomic conventional commits and the repository PR template. For releases, follow [.github/RELEASING.md](.github/RELEASING.md); default to a patch bump unless specified otherwise.

Execution profiles are source-agent policies read through `execution_policy_for_agent`; instance bindings must not create local overrides. Route queued tasks before runtime claim caches, persist selections with CAS, and keep invocation/access gates authoritative. Routing inference is a separate bounded session without task tokens; only an approved proposal can transition to execution. Preserve started-task retry selections, and require a fresh session across runtime/model changes. Saved profiles contain provider/model/OS/tool requirements, never a runtime ID. Select an authorized, ready machine for each task; release unstarted portable tasks when deleting a machine. Preserve explicit model choices during same-profile machine failover. Revalidate workspace and capabilities before starting work.

Creation drafts may include `execution_policy`. Validate it with the same provider/catalog and source-access checks as later edits, then save it inside the agent creation transaction before instance provisioning. Preserve execution profiles when storing/restoring drafts and merging AI builder output.

Agent creation presents one execution-profiles section with provider/model and capability-driven thinking/speed fields. The AI creation assistant's runtime is independent of the finished agent's profiles. Switching the assistant machine must preserve all configured profiles. Machine capabilities are refreshed independently of the UI; tools are command names and model choices come from advertised catalogs.

The AI builder receives sanitized accessible runtime catalogs and permission context on each turn. Validate proposed execution policies and scope before merging the draft, preserve omitted settings, and surface rejected proposals. Never include runtime secrets/metadata wholesale. Refresh hidden builder carrier instructions at claim time so existing sessions use the current contract. Include the builder contract directly in claimed chat input as well: large repository instruction files can exhaust CLI instruction-file budgets before the agent persona. Keep stored user messages unchanged. Omit runtime IDs from outbound profile examples. Complete JSON-only proposals may be parsed defensively, but must pass the same validation as tagged proposals. The assistant runtime is switched only through the existing locked runtime-rebind flow.

Subscription quota reports are sanitized observations, not reservations or billing authority. Poll only read-only account APIs; default tests use fake CLIs. Treat absent/stale/reset-passed values as unknown, exclude fresh exhausted applicable windows before profile selection, and preserve explicit/started selections. Scope shared account observations to the same owner and stable hashed provider identity; never infer identity from email. Custom authentication is excluded.
Claude background capacity reads the fixed Anthropic OAuth usage endpoint using the local CLI login, without inference or credential refresh/writes. Never upload credentials or provider error bodies, follow redirects with credentials, or confuse account percentages with stream-event fractions. Back off failed reads; authentication problems belong in owner-only machine logs.
Builder replies must wait for runtime/model discovery before validation. Persist applied_message_id only after successful validation and merge; unavailable catalogs are distinct from unsupported settings. Keep an explicit reapply action to recover rejected proposals without replaying over saved user edits automatically.
Execution-profile configuration validates the latest daemon-reported selectable catalog persisted before model-list completion, with provenance and a bounded age. This is separate from the live discovery cache: fallback lists may authorize picker choices but must never poison that cache or imply live provider availability. Preserve model/reasoning/speed and runtime-access validation.

Custom Foresight daemon updates use the configured instance release manifest and authenticated, fixed-platform download routes. Never send a custom build to the upstream GitHub updater. Require an advertised update protocol, owner/admin access, a newer published target, and the daemon idle/claim barrier. Verify size and SHA-256 before atomic executable replacement. Download completion is not restart confirmation: UI success requires the target version to register online. Older daemons need an installer bootstrap; do not bypass that using remote shell execution. Release artifacts stay outside version control and the Docker build context.

Background workspace preparation holds the drainable claim barrier for its entire lifetime, without incrementing the agent-task count. Server-requested updates pause new checks and bound the wait for existing checks; a timeout must release the update barrier without cancelling preparation. Actual claimed agent work still blocks replacement.

Ticket intake is a workspace-scoped policy with project overrides, stored separately from generic workspace settings. Apply it centrally in IssueService.Create only to new, unassigned, member-created, top-level, nonterminal issues. Preserve explicit assignments, Backlog parking, child issues, agent-generated work, and existing tickets. Intake leaders require an active squad, a bound runtime, and a workspace-wide invocation grant; configuration must not borrow the administrator's private-agent access. Revalidate at routing and retain normal dispatch/claim authorization. Create the intake leader task with the issue transaction, preserve media deferral, and promote only after commit with a durable fallback deadline. Instance coordinators use ordinary workspace agent bindings and workspace-local squads. Settings updates use revisions to reject stale writes.

When workspace/project ticket intake applies, the manual new-issue form hides direct assignment and submits without an assignee, including when a draft or entry point prefilled one. Wait for routing settings before allowing top-level submission; a failed settings load offers retry. Subtasks and projects with intake disabled retain manual assignment. Explicit API assignments remain supported.

AI-assisted top-level ticket creation also uses the configured intake squad. The Created by selector is replaced by the intake notice when applicable. The quick-create API resolves this policy independently of the requested author. On issue creation, only a persisted same-workspace quick-create task belonging to the authenticated author and its recorded human requester can activate intake for agent-authored work; preserve author attribution. Ordinary agent work and subtasks remain excluded to prevent recursion.
Portable quick-create agents are admitted through eligible execution candidates, never their empty saved runtime ID. Retain daemon-version and captured-context capability gates at admission and when routing/starting the selected machine. Apply the same rule to ordinary and captured-context issue creation.

Workspace repositories: `repository_configuration` stores revision-controlled workspace mappings and machine roots (machine identity = runtime owner + stable daemon ID). Workspace owners/admins manage repo/folder/mode; only machine owners manage the shared root. Workspace mappings override matching project local-directory resources and reject conflicting ones. Every claim must enforce current daemon capability, machine configuration and workspace identity. The daemon validates canonical containment, Git root and GitHub origin before execution and again before preparation; never fall back after a validation failure. Workspace repository runs use existing local-directory/worktree execution, including coordinators; in-place chat runs use the same path lock. Prompt context identifies the source repo and authorized task worktree without promising OS-level write enforcement. Workspace deletion removes only workspace-scoped repository configuration. Keep daemon capability, installer artifacts and server UI release support in sync.

Remote-update progress uses the existing authenticated update-result route and bounded append-only output, available through status polling. Preserve logs on failure and completion; late progress must not revive terminal requests. Emit only updater diagnostics, never task output, authentication headers or environment variables. Manual recovery instructions remain available offline. Windows Codex Desktop discovery searches only the current user's known application directory; an explicit executable override remains authoritative.

Workspace preparation (repository protocol v2): GitHub mappings authorize noninteractive HTTPS cloning into missing child folders of the configured existing machine root. Folder-only mappings never create directories. Daemon reports are scoped to an owned runtime or matching daemon token and verified accessible workspace, bound to workspace/machine configuration fingerprints and server timestamps. Routing and final claims require fresh readiness; unready claims are requeued. New issues require repository or folder configuration in the shared issue service and direct onboarding writers. Keep the new-issue configuration notice, machine readiness UI, source-profile routing and daemon preparation consistent. Preparation must preserve existing checkouts and keep private credentials out of reports.

Machine log snapshots are bounded (64 KiB daemon, 16 KiB crash), redacted before upload and on ingestion, and keyed by owner plus daemon identity. Only the machine owner may read them: shared runtime access and workspace admin status never grant cross-workspace log access. The CLI reads only its profile's fixed log sinks and uploads every ten seconds; no server-supplied filesystem path is accepted. Offline snapshots must retain their server receipt timestamp in the UI.
Repository preparation writes clone start/progress/completion, failures and retry schedules to the daemon logger with workspace/repository context. Capture Git stderr in a bounded buffer and redact credentials before logging; detailed Git diagnostics stay in owner-only machine logs, not workspace readiness messages.
Daemon Git checkout/worktree operations enable `core.longpaths` for the subprocess so Windows can prepare repositories with deeply nested files. Do not change global Git configuration to work around checkout failures.
New workspace clones recurse through submodules. New or updated task checkouts initialize submodules recursively at pinned commits before becoming ready; preserve kept checkouts and never force/reset submodule work or advance dependencies with `--remote`. Submodule failure blocks preparation, with bounded redacted diagnostics in machine logs.

JIRA tracking is a protected standard issue property in every workspace. The requirement defaults off and only workspace owners/admins may enable it with revision checks. Save a creation-time key in the issue transaction before intake tasks are created. Validate and normalize keys, inherit them for subtasks, and gate routing, claims and task starts when required. Preserve Multica identifiers for API operations; carry the JIRA key in claimed task context and prompts for branches, PRs and changelogs. Machines need the JIRA capability when a key is present. Do not change tracking keys during active runs.

Portable work records are workspace-scoped. Human machine-owner approval binds installation and dependent continuation to owner + stable daemon identity + provider, never a permanent agent runtime pin. Pending approval blocks later claims; offline approval must remain queued. Revalidate on routing, claim and start. Completion releases affinity only after installation verification. Agents cannot approve installations or rewrite human decisions. Branch leases use revision checks and one active task writer; checkpoint saves attest pushed SHAs, and resume must fetch and preserve divergent/local work. Bound issue context and message reads with explicit truncation and stable cursors. Detailed preparation output belongs only in redacted owner machine logs.

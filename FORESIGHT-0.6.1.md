# Foresight custom 0.6.1

Branch: `foresight-custom/0.6.1`. Upstream base: `v0.6.1` (`2ea01ae4e`).
The upgrade merge retains the complete `foresight-custom/0.4.44` history.

## Preserved custom behavior

Instance instructions and permission policies; instance agents; portable model
profiles and quota-aware routing; repository readiness; protected JIRA tracking;
installation approval with machine/provider affinity; pushed Git checkpoints and
branch leases; bounded issue context; run controls; immutable idle-safe daemon
updates and the drainable repository-preparation barrier.

The upstream generation-aware task-start and task-supplement paths now apply the
same custom execution checks. Shared-runtime supplements use the task workspace.
Fresh migrations and an upgrade of a copy of the deployed database were tested
before rollout. The old branch and pre-upgrade database backup are retained.

## Runtime capability inventory

The daemon reports a bounded, provider-specific user-scope inventory alongside
machine tools. Codex uses supported `skills/list` and `app/installed` reads without
creating an agent turn. Plugin identities are derived from provider-reported skill
paths; cached marketplace folders alone do not establish installation. Claude
reports installed user-scope plugins and their explicit enabled settings. Other
providers reuse existing local skill/MCP discovery where supported.

The UI shows enabled, disabled, unknown and stale observations. MCP configuration
and plugin installation never establish authentication. Discovery sends no config
files, command arguments, endpoint URLs, environment values or local paths.
Custom runtime wrappers and agent launch overrides do not inherit built-in reports.
Reports expire for routing after five minutes and are limited for triage prompts.

Squad triage includes per-runtime observations; execution selectors can choose an
exact approved profile/runtime pair. The server revalidates the pair when accepting
a proposal. Old profile-only proposals remain compatible. The task agent verifies
capabilities in its actual workspace/session before relying on them. Installation
still requires the existing human approval and same-machine/provider continuation.

This is advisory discovery, not automatic plugin provisioning or a guarantee that
a remote service is authenticated. Project-only plugins, plugins exposed only in a
GUI session, and Codex plugins without reported skills/apps may not be listed.
Unknown inventory never means a capability is absent. Existing custom labels that
lack translations use English fallbacks; new inventory labels cover all locales.

## Verification

Guarded daemon, agent adapter, CLI and execution package suites; custom handler and
routing regressions; generation-aware start revalidation; provider/scope/freshness
and redaction tests; frontend type checking; run-history/detail tests; inventory UI
and API parsing tests; locale key parity; production backend and web image builds.
Default tests use fake provider CLIs, never paid agent inference.

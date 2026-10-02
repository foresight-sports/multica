-- name: GetInstanceConfiguration :one
SELECT * FROM instance_configuration WHERE singleton = true;

-- name: UpdateInstanceConfiguration :one
UPDATE instance_configuration SET instructions = @instructions, updated_by = @updated_by,
    revision = revision + 1, updated_at = now()
WHERE singleton = true AND revision = @expected_revision
RETURNING *;

-- name: ListInstanceAgents :many
SELECT * FROM instance_agent ORDER BY name, id;

-- name: GetInstanceAgent :one
SELECT * FROM instance_agent WHERE id = $1;

-- name: ListInstanceAgentBindings :many
SELECT * FROM agent WHERE instance_agent_id = $1;

-- name: CreateInstanceAgent :one
INSERT INTO instance_agent (name, description, instructions, created_by)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: LinkInstanceAgent :one
WITH definition AS (
    UPDATE instance_agent SET source_agent_id = @id WHERE id = @instance_agent_id RETURNING source_agent_id
)
UPDATE agent SET instance_agent_id = @instance_agent_id, instance_source_agent_id = definition.source_agent_id,
    name = name || ' (Instance)', updated_at = now()
FROM definition WHERE agent.id = @id AND workspace_id = @workspace_id AND instance_agent_id IS NULL
RETURNING agent.*;

-- name: UpdateInstanceAgent :one
UPDATE instance_agent SET name = @name, description = @description,
    instructions = @instructions, revision = revision + 1, updated_at = now()
WHERE id = @id AND revision = @expected_revision RETURNING *;

-- name: ProvisionInstanceAgents :many
-- Workspace rows are execution bindings. Identity and instructions are managed
-- centrally; runtimes, skills, access and history remain workspace-specific.
-- Never update an existing binding here: archived rows are explicit opt-outs.
INSERT INTO agent (workspace_id, instance_agent_id, instance_source_agent_id, name, description, instructions,
    runtime_mode, runtime_config, visibility, permission_mode, max_concurrent_tasks,
    owner_id, custom_env, custom_args)
SELECT w.id, i.id, i.source_agent_id, i.name || ' (Instance)',
    i.description, i.instructions, 'local', '{}'::jsonb, 'workspace', 'public_to', 6,
    m.user_id, '{}'::jsonb, '[]'::jsonb
FROM workspace w CROSS JOIN instance_agent i
JOIN LATERAL (SELECT user_id FROM member WHERE workspace_id = w.id AND role = 'owner'
    ORDER BY created_at, id LIMIT 1) m ON true
WHERE (sqlc.narg('workspace_id')::uuid IS NULL OR w.id = sqlc.narg('workspace_id')::uuid)
ON CONFLICT (workspace_id, instance_agent_id) DO NOTHING
RETURNING *;

-- name: UpdateInstanceAgentBindings :many
-- Materialized identity fields keep existing assignment/search surfaces working.
-- Updates are committed with the definition; workspace opt-outs are preserved.
UPDATE agent SET name = @name || ' (Instance)',
    description = @description, instructions = @instructions, updated_at = now()
WHERE instance_agent_id = @instance_agent_id RETURNING *;

-- name: InitializeInstanceAgentSources :exec
WITH initialized AS (
    UPDATE instance_agent i SET source_agent_id = (
        SELECT a.id FROM agent a WHERE a.instance_agent_id = i.id ORDER BY a.created_at, a.id LIMIT 1
    ) WHERE source_agent_id IS NULL RETURNING id, source_agent_id
)
UPDATE agent a SET instance_source_agent_id = i.source_agent_id
FROM initialized i WHERE a.instance_agent_id = i.id;

-- name: SyncInstanceAgentConfigurations :exec
SELECT sync_instance_agent_configuration(source_agent_id) FROM instance_agent WHERE source_agent_id IS NOT NULL;

-- name: InstanceTaskWorkspace :one
-- An explicit central runtime grant, never a blanket cross-workspace bypass.
SELECT a.workspace_id FROM agent a
JOIN instance_agent i ON i.id = a.instance_agent_id AND i.source_agent_id = a.instance_source_agent_id
JOIN agent source ON source.id = i.source_agent_id AND source.instance_agent_id = i.id
WHERE a.id = @agent_id AND agent_allows_runtime(a.id, @runtime_id);

-- name: InstanceAgentWorkspaces :one
-- Lifecycle callbacks remain authorized after a shared runtime is rebound or
-- removed, so cancellation acknowledgements can drain the old execution.
SELECT a.workspace_id, source.workspace_id AS source_workspace_id FROM agent a
JOIN instance_agent i ON i.id = a.instance_agent_id AND i.source_agent_id = a.instance_source_agent_id
JOIN agent source ON source.id = i.source_agent_id AND source.instance_agent_id = i.id
WHERE a.id = $1;

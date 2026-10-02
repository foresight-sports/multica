-- name: GetRepositoryConfiguration :one
SELECT config, revision FROM repository_configuration WHERE scope = @scope AND subject = @subject;

-- name: SaveRepositoryConfiguration :one
INSERT INTO repository_configuration(scope, subject, config, revision)
SELECT @scope, @subject, @config::jsonb, 1 WHERE @expected_revision::bigint = 0
ON CONFLICT (scope, subject) DO UPDATE SET config = EXCLUDED.config, revision = repository_configuration.revision + 1
WHERE repository_configuration.revision = @expected_revision
RETURNING config, revision;

-- name: UpdateRepositoryConfiguration :one
UPDATE repository_configuration SET config = @config::jsonb, revision = revision + 1
WHERE scope = @scope AND subject = @subject AND revision = @expected_revision
RETURNING config, revision;

-- name: ListRuntimeRepositoryWorkspaces :many
SELECT id FROM workspace WHERE id = (SELECT workspace_id FROM agent_runtime WHERE id = sqlc.arg(runtime_id)::uuid)
OR EXISTS (SELECT 1 FROM agent a WHERE a.workspace_id=workspace.id AND agent_allows_runtime(a.id, sqlc.arg(runtime_id)::uuid));

-- name: PutRepositoryReadiness :exec
INSERT INTO repository_configuration(scope,subject,config,revision)
VALUES ('readiness',@subject,@config::jsonb,1)
ON CONFLICT(scope,subject) DO UPDATE SET config=EXCLUDED.config,revision=repository_configuration.revision+1;

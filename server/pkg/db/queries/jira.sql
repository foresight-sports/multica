-- name: GetJiraProperty :one
SELECT * FROM issue_property WHERE workspace_id=$1 AND config->>'system_key'='jira_ticket_id' LIMIT 1;

-- name: SaveJiraRequirement :one
UPDATE issue_property SET config=config || jsonb_build_object('required',sqlc.arg(required)::boolean,'revision',sqlc.arg(revision)::bigint+1),updated_at=now()
WHERE workspace_id=sqlc.arg(workspace_id)::uuid AND config->>'system_key'='jira_ticket_id'
  AND (config->>'revision')::bigint=sqlc.arg(revision)::bigint RETURNING *;

-- name: GetIssueJiraKey :one
SELECT COALESCE(issue_jira_key($1::uuid),'')::text AS jira_ticket_id;

-- name: TaskJiraAllowed :one
SELECT task_jira_allowed($1::uuid,$2::uuid,$3::jsonb)::boolean AS allowed;

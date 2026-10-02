-- name: GetIssueIntake :one
SELECT issue_intake FROM workspace WHERE id=$1;

-- name: UpdateIssueIntake :one
UPDATE workspace SET issue_intake= @config::jsonb,updated_at=now()
WHERE id= @workspace_id AND COALESCE((issue_intake->>'revision')::int,0)= @revision::int
RETURNING issue_intake;

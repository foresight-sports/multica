-- name: GetPermissionPolicy :one
SELECT * FROM permission_policy WHERE action = $1;

-- name: CreatePermissionPolicy :one
INSERT INTO permission_policy (action, allowed_emails, updated_by)
VALUES ($1, $2, $3)
ON CONFLICT (action) DO NOTHING
RETURNING *;

-- name: UpdatePermissionPolicy :one
UPDATE permission_policy
SET allowed_emails = @allowed_emails, updated_by = @updated_by,
    updated_at = now(), revision = revision + 1
WHERE action = @action AND revision = @expected_revision
RETURNING *;

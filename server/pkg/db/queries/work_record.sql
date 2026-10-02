-- name: CreateWorkRecord :one
INSERT INTO work_record(id,workspace_id,issue_id,agent_id,task_id,kind,resource_key,state,machine_owner_id,daemon_id,runtime_id,data)
VALUES (@id,@workspace_id,@issue_id,sqlc.narg(agent_id),sqlc.narg(task_id),@kind,@resource_key,@state,sqlc.narg(machine_owner_id),@daemon_id,sqlc.narg(runtime_id),@data::jsonb) RETURNING *;

-- name: GetWorkRecord :one
SELECT * FROM work_record WHERE id= @id AND workspace_id= @workspace_id;

-- name: LockWorkRecord :one
SELECT * FROM work_record WHERE id= @id AND workspace_id= @workspace_id FOR UPDATE;

-- name: GetBranchWorkRecord :one
SELECT * FROM work_record WHERE workspace_id= @workspace_id AND kind='checkpoint' AND resource_key= @resource_key;

-- name: ListContextComments :many
SELECT * FROM (SELECT id,parent_id,author_type,author_id,content,created_at FROM comment
WHERE issue_id= @issue_id AND workspace_id= @workspace_id AND deleted_at IS NULL
AND (created_at,id) > (@since::timestamptz,@after_id::uuid) AND created_at <= @until::timestamptz
ORDER BY CASE WHEN @recent::boolean THEN created_at END DESC, CASE WHEN @recent::boolean THEN id END DESC,created_at,id LIMIT 51) bounded ORDER BY created_at,id;

-- name: ListContextTasks :many
SELECT * FROM agent_task_queue WHERE issue_id= @issue_id ORDER BY created_at DESC LIMIT 51;

-- name: ListWorkRecords :many
SELECT * FROM work_record WHERE workspace_id= @workspace_id AND issue_id= @issue_id ORDER BY (state IN ('resolved','completed','declined','cancelled')),updated_at DESC,id LIMIT 201;

-- name: UpdateWorkRecord :one
UPDATE work_record SET state= @state,data= @data::jsonb,revision=revision+1,updated_at=now(),
 approved_by=sqlc.narg(approved_by),continuation_task_id=sqlc.narg(continuation_task_id),
 task_id=sqlc.narg(task_id),agent_id=sqlc.narg(agent_id)
WHERE id= @id AND workspace_id= @workspace_id AND revision= @revision RETURNING *;

-- name: TaskWorkflowAllowed :one
SELECT task_workflow_allowed(@agent_id::uuid,sqlc.narg(issue_id)::uuid,sqlc.narg(runtime_id)::uuid)::boolean AS allowed;

-- name: FinishInstalledWorkRecords :exec
UPDATE work_record w SET state='completed',revision=revision+1,updated_at=now()
FROM agent_task_queue t,agent_runtime r
WHERE t.id= @task_id AND t.status='completed' AND w.kind='installation' AND w.state='installed'
AND w.issue_id=t.issue_id AND w.agent_id=t.agent_id AND t.runtime_id=r.id
AND r.owner_id=w.machine_owner_id AND r.daemon_id=w.daemon_id
AND w.continuation_task_id IS NOT NULL AND t.created_at >= (SELECT created_at FROM agent_task_queue WHERE id=w.continuation_task_id);

-- name: DeleteWorkspaceWorkRecords :exec
DELETE FROM work_record WHERE workspace_id=$1;

-- name: ListActiveWorkRecords :many
SELECT * FROM work_record WHERE workspace_id= @workspace_id AND issue_id= @issue_id
AND state NOT IN ('resolved','completed','declined','cancelled') ORDER BY updated_at DESC,id LIMIT 201;

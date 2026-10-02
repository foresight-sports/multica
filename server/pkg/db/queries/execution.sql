-- name: GetExecutionPolicy :one
SELECT execution_policy_for_agent($1)::jsonb AS policy;

-- name: SaveExecutionPolicy :one
UPDATE agent SET execution_policy= @policy::jsonb,updated_at=now()
WHERE id= @id AND COALESCE((execution_policy->>'revision')::bigint,0)= @expected_revision::bigint
RETURNING *;

-- name: AgentAllowsRuntime :one
SELECT agent_allows_runtime(@agent_id,@runtime_id)::boolean AS allowed;

-- name: ListExecutionRoutingTasks :many
SELECT t.* FROM agent_task_queue t
WHERE t.status IN ('queued','deferred') AND t.started_at IS NULL
AND COALESCE(jsonb_array_length(execution_policy_for_agent(t.agent_id)->'profiles'),0)>0
AND EXISTS (SELECT 1 FROM unnest(@runtime_ids::text[]) rid WHERE agent_allows_runtime(t.agent_id,rid::uuid))
ORDER BY t.created_at;

-- name: SetTaskExecution :one
UPDATE agent_task_queue SET runtime_id= @runtime_id, execution_selection= @selection::jsonb,
 execution_request= @request::jsonb,
 handoff_note=CASE WHEN @fresh_session::boolean AND handoff_note IS NULL THEN 'Execution profile selected for a fresh session. Read the supplied issue or chat context before continuing; do not assume access to a previous runtime session or uncommitted files.' ELSE handoff_note END,
 force_fresh_session=force_fresh_session OR @fresh_session::boolean
WHERE id= @id AND status IN ('queued','deferred') AND started_at IS NULL
AND execution_selection= @expected_selection::jsonb
RETURNING *;

-- name: ResolveTaskExecution :one
UPDATE agent_task_queue SET runtime_id= @runtime_id, execution_selection= @selection::jsonb,
 status='queued',dispatched_at=NULL,prepare_lease_expires_at=NULL,delivered_comment_ids='{}',
 force_fresh_session=force_fresh_session OR @fresh_session::boolean
WHERE id= @id AND runtime_id= @old_runtime_id AND status='dispatched' AND started_at IS NULL
AND execution_selection->>'state'='selecting' AND dispatched_at= @dispatched_at
RETURNING *;

-- name: GetPreviousExecution :one
SELECT t.* FROM agent_task_queue t WHERE t.agent_id= @agent_id AND t.id<> @id
AND t.started_at IS NOT NULL AND t.execution_selection->>'state'='selected'
AND ((sqlc.narg(issue_id)::uuid IS NOT NULL AND t.issue_id=sqlc.narg(issue_id))
 OR (sqlc.narg(chat_session_id)::uuid IS NOT NULL AND t.chat_session_id=sqlc.narg(chat_session_id)))
ORDER BY t.created_at DESC LIMIT 1;

-- name: SetExecutionCatalog :exec
UPDATE agent_runtime SET metadata=COALESCE(metadata,'{}') || jsonb_build_object('execution_models',@models::jsonb) WHERE id= @id;
-- name: SetExecutionCapabilities :exec
UPDATE agent_runtime SET metadata=COALESCE(metadata,'{}') || @capabilities::jsonb WHERE id= @id;

-- name: RuntimeHasExecutionProfiles :one
SELECT EXISTS(SELECT 1 FROM agent WHERE EXISTS(SELECT 1 FROM jsonb_array_elements(COALESCE(execution_policy->'profiles','[]')) p WHERE p->>'runtime_id'=$1::uuid::text))::boolean;
-- name: AgentHasProfiledWork :one
SELECT EXISTS(SELECT 1 FROM agent_task_queue t JOIN agent a ON a.id=t.agent_id
WHERE COALESCE(a.instance_source_agent_id,a.id)=$1 AND t.status IN ('queued','deferred','dispatched','running','waiting_local_directory') AND t.execution_selection<>'{}'::jsonb)::boolean;

-- name: SetSubscriptionQuota :exec
UPDATE agent_runtime SET metadata=COALESCE(metadata,'{}') || jsonb_build_object('subscription_quota',@report::jsonb)
WHERE id= @id AND COALESCE((metadata->'subscription_quota'->>'observed_at')::timestamptz,'-infinity') <= (@report::jsonb->>'observed_at')::timestamptz;

-- name: GetSubscriptionQuota :one
SELECT COALESCE((SELECT candidate.metadata->'subscription_quota'
 FROM agent_runtime target JOIN agent_runtime candidate ON candidate.id=target.id
 OR (candidate.owner_id=target.owner_id AND candidate.provider=target.provider
 AND NULLIF(target.metadata->'subscription_quota'->>'account_key','') IS NOT NULL
 AND candidate.metadata->'subscription_quota'->>'account_key'=target.metadata->'subscription_quota'->>'account_key')
 WHERE target.id= @id AND candidate.metadata->'subscription_quota' IS NOT NULL
 ORDER BY (candidate.metadata->'subscription_quota'->>'observed_at')::timestamptz DESC LIMIT 1),'{}'::jsonb)::jsonb AS report;

-- name: ListAgentExecutionRuntimes :many
SELECT r.* FROM agent_runtime r WHERE agent_allows_runtime(sqlc.arg(agent_id)::uuid,r.id)
ORDER BY (SELECT count(*) FROM agent_task_queue t WHERE t.runtime_id=r.id AND t.status IN ('dispatched','running')), r.id;

-- name: ReleasePortableTasksFromRuntime :exec
UPDATE agent_task_queue SET runtime_id=NULL,
 execution_selection=jsonb_build_object('state','pending','history',COALESCE(execution_selection->'history','[]'::jsonb)),
 force_fresh_session=true
WHERE runtime_id=$1 AND status IN ('queued','deferred') AND started_at IS NULL
 AND COALESCE(execution_selection->>'locked','false')<>'true'
 AND COALESCE(execution_request->>'runtime_id','')=''
 AND COALESCE(jsonb_array_length(execution_policy_for_agent(agent_id)->'profiles'),0)>0;

-- name: ListRuntimeRequiredTools :many
SELECT DISTINCT tool.value::text AS name FROM agent a,
 jsonb_array_elements(COALESCE(execution_policy_for_agent(a.id)->'profiles','[]')) p,
 jsonb_array_elements_text(COALESCE(p->'required_tools','[]')) tool
WHERE agent_allows_runtime(a.id,$1::uuid) ORDER BY name LIMIT 80;

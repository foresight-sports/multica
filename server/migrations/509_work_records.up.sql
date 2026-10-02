CREATE TABLE work_record (
 id uuid NOT NULL,
 workspace_id uuid NOT NULL,
 issue_id uuid NOT NULL,
 agent_id uuid,
 task_id uuid,
 kind text NOT NULL CHECK (kind IN ('installation','checkpoint','decision','blocker')),
 resource_key text NOT NULL DEFAULT '',
 state text NOT NULL,
 machine_owner_id uuid,
 daemon_id text NOT NULL DEFAULT '',
 runtime_id uuid,
 approved_by uuid,
 continuation_task_id uuid,
 data jsonb NOT NULL DEFAULT '{}',
 revision bigint NOT NULL DEFAULT 1,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE FUNCTION task_workflow_allowed(task_agent uuid, task_issue uuid, task_runtime uuid)
RETURNS boolean LANGUAGE sql STABLE AS $$
 SELECT NOT EXISTS (
  SELECT 1 FROM work_record w WHERE w.kind='installation'
   AND w.issue_id=task_issue AND w.agent_id=task_agent
   AND w.state IN ('pending','approved','installed')
   AND (w.state='pending' OR NOT EXISTS (
    SELECT 1 FROM agent_runtime r WHERE r.id=task_runtime
     AND r.owner_id=w.machine_owner_id AND r.daemon_id=w.daemon_id
     AND r.provider=(w.data->>'provider')
   ))
 );
$$;

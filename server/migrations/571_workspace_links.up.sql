CREATE TABLE workspace_link (
 workspace_a uuid NOT NULL, workspace_b uuid NOT NULL,
 enabled boolean NOT NULL DEFAULT true, updated_by uuid NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(workspace_a,workspace_b), CHECK(workspace_a < workspace_b)
);
CREATE TABLE workspace_request (
 id uuid PRIMARY KEY, source_issue_id uuid NOT NULL, target_issue_id uuid NOT NULL UNIQUE,
 source_workspace_id uuid NOT NULL, target_workspace_id uuid NOT NULL,
 requester_id uuid NOT NULL, request_key text NOT NULL,
 last_signature text NOT NULL DEFAULT '', last_task_id uuid,
 last_error text NOT NULL DEFAULT '', checked_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(source_issue_id,request_key), CHECK(source_workspace_id <> target_workspace_id)
);

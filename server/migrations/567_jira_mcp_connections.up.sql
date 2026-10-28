CREATE TABLE IF NOT EXISTS jira_mcp_connection (
 user_id uuid PRIMARY KEY,
 credentials bytea,
 expires_at timestamptz,
 status text NOT NULL DEFAULT 'disconnected',
 pending_state text,
 pending_credentials bytea,
 pending_until timestamptz,
 updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS jira_mcp_session (
 id text PRIMARY KEY,
 task_id uuid NOT NULL,
 user_id uuid NOT NULL,
 upstream_id bytea NOT NULL,
 expires_at timestamptz NOT NULL DEFAULT now()+interval '24 hours'
);

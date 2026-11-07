CREATE TABLE slack_relay_health (
 installation_id uuid NOT NULL, workspace_id uuid NOT NULL, last_seen_at timestamptz NOT NULL DEFAULT now()
);

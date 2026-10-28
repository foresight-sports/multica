CREATE TABLE IF NOT EXISTS provider_auth_requests (
    machine_key text PRIMARY KEY,
    id uuid NOT NULL,
    runtime_id uuid NOT NULL,
    provider text NOT NULL CHECK (provider IN ('claude', 'codex')),
    status text NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL DEFAULT now() + interval '2 minutes'
);

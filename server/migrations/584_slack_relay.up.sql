CREATE TABLE slack_relay_receipt (
 id uuid NOT NULL, installation_id uuid NOT NULL, workspace_id uuid NOT NULL,
 channel_id text NOT NULL, message_ts text NOT NULL, rule_id uuid,
 owner text NOT NULL CHECK(owner IN ('onyx','multica')),
 state text NOT NULL CHECK(state IN ('pending','delivered','ignored','held_disabled','waiting_root','failed','onyx')),
 payload jsonb NOT NULL DEFAULT '{}', attempts integer NOT NULL DEFAULT 0,
 last_error text NOT NULL DEFAULT '', next_attempt_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 last_received_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE slack_relay_nonce (
 installation_id uuid NOT NULL, nonce text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);

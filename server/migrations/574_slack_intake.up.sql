CREATE TABLE automation_account (
 user_id uuid PRIMARY KEY, workspace_id uuid NOT NULL, created_by uuid NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE slack_intake_rule (
 id uuid PRIMARY KEY, workspace_id uuid NOT NULL, installation_id uuid NOT NULL,
 channel_id text NOT NULL, channel_name text NOT NULL, workflow text NOT NULL DEFAULT 'support_diagnosis',
 service_user_id uuid NOT NULL, coordinator_id uuid NOT NULL, analyst_id uuid NOT NULL,
 composer_id uuid, audience text NOT NULL DEFAULT 'customer_support', style_instructions text NOT NULL DEFAULT '',
 escalation_user_id text NOT NULL, enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1, timeout_minutes integer NOT NULL DEFAULT 60,
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(),
 CHECK(workflow='support_diagnosis'),
 CHECK(timeout_minutes BETWEEN 5 AND 1440)
);
CREATE TABLE slack_intake_thread (
 id uuid PRIMARY KEY, rule_id uuid NOT NULL, workspace_id uuid NOT NULL,
 root_ts text NOT NULL, issue_id uuid, backlog_issue_id uuid,
 revision bigint NOT NULL DEFAULT 0, queued_revision bigint NOT NULL DEFAULT 0,
 state text NOT NULL DEFAULT 'pending', last_error text NOT NULL DEFAULT '',
 reaction_active boolean NOT NULL DEFAULT false, checked_at timestamptz NOT NULL DEFAULT now(),
 created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE slack_intake_message (
 id uuid PRIMARY KEY, thread_id uuid NOT NULL, rule_id uuid NOT NULL, message_ts text NOT NULL,
 sender_id text NOT NULL, body text NOT NULL, raw jsonb NOT NULL DEFAULT '{}',
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE slack_intake_reply (
 id uuid PRIMARY KEY, thread_id uuid NOT NULL, revision bigint NOT NULL, task_id uuid,
 outcome text NOT NULL, confidence double precision NOT NULL, body text NOT NULL,
 evidence text NOT NULL, state text NOT NULL DEFAULT 'pending', slack_ts text,
 last_error text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 CHECK(outcome IN ('answer','clarify','defect','escalate')),
 CHECK(confidence BETWEEN 0 AND 1)
);

CREATE TABLE instance_configuration (
    singleton BOOLEAN NOT NULL DEFAULT true CHECK (singleton),
    instructions TEXT NOT NULL DEFAULT '',
    revision BIGINT NOT NULL DEFAULT 1,
    updated_by UUID,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
INSERT INTO instance_configuration (singleton) VALUES (true);

CREATE TABLE instance_agent (
    id UUID NOT NULL DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    instructions TEXT NOT NULL DEFAULT '',
    revision BIGINT NOT NULL DEFAULT 1,
    created_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
ALTER TABLE agent ADD COLUMN instance_agent_id UUID;

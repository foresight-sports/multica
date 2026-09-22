CREATE TABLE permission_policy (
    action TEXT NOT NULL,
    allowed_emails TEXT[] NOT NULL DEFAULT '{}',
    revision BIGINT NOT NULL DEFAULT 1,
    updated_by UUID NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

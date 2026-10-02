ALTER TABLE workspace ADD COLUMN IF NOT EXISTS issue_intake jsonb NOT NULL DEFAULT '{}'::jsonb;

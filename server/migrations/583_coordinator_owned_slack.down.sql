UPDATE slack_intake_rule SET analyst_id=coordinator_id WHERE analyst_id IS NULL;
ALTER TABLE slack_intake_rule ALTER COLUMN analyst_id SET NOT NULL;
ALTER TABLE instance_configuration DROP COLUMN audience_instructions;

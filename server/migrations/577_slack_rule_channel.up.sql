CREATE UNIQUE INDEX CONCURRENTLY slack_rule_channel_unique ON slack_intake_rule (installation_id,channel_id);

CREATE UNIQUE INDEX CONCURRENTLY slack_message_receipt_unique ON slack_intake_message (rule_id,message_ts);

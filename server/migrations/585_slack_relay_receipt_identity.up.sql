CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS slack_relay_receipt_identity ON slack_relay_receipt(installation_id, channel_id, message_ts);

CREATE INDEX CONCURRENTLY IF NOT EXISTS slack_relay_poll ON slack_relay_receipt(next_attempt_at, message_ts) WHERE state IN ('pending','waiting_root');

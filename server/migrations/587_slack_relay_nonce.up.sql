CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS slack_relay_nonce_identity ON slack_relay_nonce(installation_id, nonce);

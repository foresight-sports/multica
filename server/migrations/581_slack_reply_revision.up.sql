CREATE UNIQUE INDEX CONCURRENTLY slack_reply_revision_unique ON slack_intake_reply (thread_id,revision);

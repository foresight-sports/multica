CREATE UNIQUE INDEX CONCURRENTLY slack_thread_root_unique ON slack_intake_thread (rule_id,root_ts);

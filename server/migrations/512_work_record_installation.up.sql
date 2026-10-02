CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS work_record_installation_idx ON work_record(issue_id,agent_id) WHERE kind='installation' AND state IN ('pending','approved','installed');

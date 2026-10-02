CREATE INDEX CONCURRENTLY IF NOT EXISTS work_record_issue_idx ON work_record(workspace_id,issue_id,updated_at,id);

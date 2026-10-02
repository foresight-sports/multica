CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS work_record_branch_idx ON work_record(workspace_id,resource_key) WHERE kind='checkpoint';

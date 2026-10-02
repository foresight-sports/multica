CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS agent_instance_workspace_unique ON agent (workspace_id, instance_agent_id);

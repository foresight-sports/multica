CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS instance_agent_name_unique ON instance_agent (lower(name));

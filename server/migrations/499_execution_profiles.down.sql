DROP TRIGGER IF EXISTS task_execution_initialize ON agent_task_queue;
DROP FUNCTION IF EXISTS initialize_task_execution();
DROP FUNCTION IF EXISTS agent_allows_runtime(UUID,UUID);
DROP FUNCTION IF EXISTS execution_policy_for_agent(UUID);
ALTER TABLE agent_task_queue DROP COLUMN IF EXISTS execution_selection;
ALTER TABLE agent_task_queue DROP COLUMN IF EXISTS execution_request;
ALTER TABLE agent DROP COLUMN IF EXISTS execution_policy;

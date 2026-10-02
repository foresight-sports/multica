DROP TRIGGER IF EXISTS issue_jira_key ON issue;
DROP FUNCTION IF EXISTS maintain_issue_jira_key();
DROP TRIGGER IF EXISTS jira_task_start ON agent_task_queue;
DROP FUNCTION IF EXISTS enforce_jira_task_start();
DROP FUNCTION IF EXISTS task_jira_allowed(uuid,uuid,jsonb);
DROP FUNCTION IF EXISTS issue_jira_key(uuid);
DROP TRIGGER IF EXISTS workspace_jira_property ON workspace;
DROP FUNCTION IF EXISTS workspace_jira_property_trigger();
DROP FUNCTION IF EXISTS provision_jira_property(uuid);
-- Preserve definitions and ticket values on rollback.

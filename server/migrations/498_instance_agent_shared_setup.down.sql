DROP TRIGGER IF EXISTS instance_agent_access_changed ON agent_invocation_target;
DROP TRIGGER IF EXISTS instance_agent_skills_changed ON agent_skill;
DROP TRIGGER IF EXISTS instance_agent_setup_changed ON agent;
DROP FUNCTION IF EXISTS sync_instance_agent_relation();
DROP FUNCTION IF EXISTS sync_instance_agent_row();
DROP FUNCTION IF EXISTS sync_instance_agent_configuration(UUID);
ALTER TABLE agent DROP COLUMN IF EXISTS instance_source_agent_id;
ALTER TABLE instance_agent DROP COLUMN IF EXISTS source_agent_id;

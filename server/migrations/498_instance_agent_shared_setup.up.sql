ALTER TABLE instance_agent ADD COLUMN source_agent_id UUID;
ALTER TABLE agent ADD COLUMN instance_source_agent_id UUID;

UPDATE instance_agent i SET source_agent_id = (
    SELECT a.id FROM agent a WHERE a.instance_agent_id = i.id
    ORDER BY (a.runtime_id IS NOT NULL) DESC, a.created_at, a.id LIMIT 1
);
UPDATE agent a SET instance_source_agent_id = i.source_agent_id
FROM instance_agent i WHERE a.instance_agent_id = i.id;

-- Keep the normal agent editor as the single configuration owner. Replicas
-- retain their workspace, archive state and history, but never local settings.
CREATE FUNCTION sync_instance_agent_configuration(source_id UUID) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE source agent%ROWTYPE;
BEGIN
    SELECT * INTO source FROM agent WHERE id = source_id
        AND instance_source_agent_id = id FOR UPDATE;
    IF NOT FOUND THEN RETURN; END IF;
    UPDATE agent SET runtime_id = source.runtime_id, runtime_mode = source.runtime_mode,
        runtime_config = source.runtime_config, owner_id = source.owner_id,
        model = source.model, thinking_level = source.thinking_level, service_tier = source.service_tier,
        avatar_url = source.avatar_url, custom_env = source.custom_env, custom_args = source.custom_args,
        mcp_config = source.mcp_config, conversation_starters = source.conversation_starters,
        composio_toolkit_allowlist = source.composio_toolkit_allowlist,
        disabled_runtime_skills = source.disabled_runtime_skills,
        max_concurrent_tasks = source.max_concurrent_tasks,
        permission_mode = source.permission_mode, visibility = source.visibility, updated_at = now()
    WHERE instance_source_agent_id = source_id AND id <> source_id;
    DELETE FROM agent_skill WHERE agent_id IN (
        SELECT id FROM agent WHERE instance_source_agent_id = source_id AND id <> source_id);
    INSERT INTO agent_skill (agent_id, skill_id, enabled)
    SELECT a.id, s.skill_id, s.enabled FROM agent a CROSS JOIN agent_skill s
    WHERE a.instance_source_agent_id = source_id AND a.id <> source_id AND s.agent_id = source_id;
    DELETE FROM agent_invocation_target WHERE agent_id IN (
        SELECT id FROM agent WHERE instance_source_agent_id = source_id AND id <> source_id);
    INSERT INTO agent_invocation_target (agent_id, target_type, target_id, created_by)
    SELECT a.id, t.target_type,
        CASE WHEN t.target_type = 'workspace' THEN a.workspace_id ELSE t.target_id END, t.created_by
    FROM agent a CROSS JOIN agent_invocation_target t
    WHERE a.instance_source_agent_id = source_id AND a.id <> source_id AND t.agent_id = source_id;
END;
$$;

CREATE FUNCTION sync_instance_agent_row() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    -- Runtime teardown already unbinds every affected agent in one statement.
    -- Updating those rows again from a row trigger would conflict with that
    -- statement. Reconciliation also handles any source-only unbind.
    IF OLD.runtime_id IS NOT NULL AND NEW.runtime_id IS NULL THEN RETURN NEW; END IF;
    IF NEW.instance_source_agent_id = NEW.id THEN
        PERFORM sync_instance_agent_configuration(NEW.id);
    END IF;
    RETURN NEW;
END;
$$;
CREATE TRIGGER instance_agent_setup_changed AFTER UPDATE OF
    runtime_id, runtime_mode, runtime_config, owner_id, model, thinking_level, service_tier,
    avatar_url, custom_env, custom_args, mcp_config, conversation_starters,
    composio_toolkit_allowlist, disabled_runtime_skills, max_concurrent_tasks, permission_mode, visibility
ON agent FOR EACH ROW EXECUTE FUNCTION sync_instance_agent_row();

CREATE FUNCTION sync_instance_agent_relation() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE source_id UUID;
BEGIN
    IF TG_OP = 'DELETE' THEN source_id := OLD.agent_id; ELSE source_id := NEW.agent_id; END IF;
    IF EXISTS (SELECT 1 FROM agent WHERE id = source_id AND instance_source_agent_id = id) THEN
        PERFORM sync_instance_agent_configuration(source_id);
    END IF;
    RETURN NULL;
END;
$$;
CREATE TRIGGER instance_agent_skills_changed AFTER INSERT OR UPDATE OR DELETE ON agent_skill
FOR EACH ROW EXECUTE FUNCTION sync_instance_agent_relation();
CREATE TRIGGER instance_agent_access_changed AFTER INSERT OR UPDATE OR DELETE ON agent_invocation_target
FOR EACH ROW EXECUTE FUNCTION sync_instance_agent_relation();

SELECT sync_instance_agent_configuration(source_agent_id) FROM instance_agent WHERE source_agent_id IS NOT NULL;

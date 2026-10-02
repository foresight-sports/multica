ALTER TABLE agent_task_queue DROP CONSTRAINT IF EXISTS agent_task_queue_active_requires_runtime;
ALTER TABLE agent_task_queue ADD CONSTRAINT agent_task_queue_active_requires_runtime
CHECK (runtime_id IS NOT NULL OR completed_at IS NOT NULL OR
 (status IN ('queued','deferred') AND execution_selection->>'state' IN ('pending','blocked'))) NOT VALID;

-- Bindings cache requirements for admission/UI; routing always reads the source.
CREATE FUNCTION sync_portable_execution_policy() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.instance_source_agent_id=NEW.id THEN
  UPDATE agent SET execution_policy=NEW.execution_policy WHERE instance_source_agent_id=NEW.id AND id<>NEW.id;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER portable_execution_sync AFTER UPDATE OF execution_policy ON agent
FOR EACH ROW EXECUTE FUNCTION sync_portable_execution_policy();

CREATE FUNCTION initialize_portable_agent() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF NEW.instance_source_agent_id IS NOT NULL AND NEW.instance_source_agent_id<>NEW.id THEN
  SELECT execution_policy INTO NEW.execution_policy FROM agent WHERE id=NEW.instance_source_agent_id;
 END IF;
 IF COALESCE(jsonb_array_length(NEW.execution_policy->'profiles'),0)>0 THEN NEW.runtime_id=NULL; END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER portable_agent_initialize BEFORE INSERT OR UPDATE OF execution_policy, runtime_id ON agent
FOR EACH ROW EXECUTE FUNCTION initialize_portable_agent();

UPDATE agent a SET execution_policy=source.execution_policy FROM agent source
WHERE a.instance_source_agent_id=source.id AND a.id<>source.id;
UPDATE agent SET runtime_id=NULL WHERE COALESCE(jsonb_array_length(execution_policy->'profiles'),0)>0;

ALTER TABLE agent ADD COLUMN execution_policy JSONB NOT NULL DEFAULT '{}';
ALTER TABLE agent_task_queue ADD COLUMN execution_request JSONB NOT NULL DEFAULT '{}';
ALTER TABLE agent_task_queue ADD COLUMN execution_selection JSONB NOT NULL DEFAULT '{}';

CREATE FUNCTION execution_policy_for_agent(p_agent_id UUID) RETURNS JSONB LANGUAGE sql STABLE AS $$
 SELECT source.execution_policy FROM agent a JOIN agent source
 ON source.id=COALESCE(a.instance_source_agent_id,a.id)
 WHERE a.id=p_agent_id AND (a.instance_agent_id IS NULL OR EXISTS (
 SELECT 1 FROM instance_agent i WHERE i.id=a.instance_agent_id AND i.source_agent_id=source.id AND source.instance_agent_id=i.id))
$$;

CREATE FUNCTION agent_allows_runtime(p_agent_id UUID, p_runtime_id UUID) RETURNS BOOLEAN LANGUAGE sql STABLE AS $$
 SELECT EXISTS (SELECT 1 FROM agent a JOIN agent source ON source.id=COALESCE(a.instance_source_agent_id,a.id)
 JOIN agent_runtime r ON r.id=p_runtime_id
 WHERE a.id=p_agent_id AND (a.instance_agent_id IS NULL OR EXISTS (
 SELECT 1 FROM instance_agent i WHERE i.id=a.instance_agent_id AND i.source_agent_id=source.id AND source.instance_agent_id=i.id))
 AND r.workspace_id=source.workspace_id
 AND (a.runtime_id=r.id OR EXISTS (SELECT 1 FROM jsonb_array_elements(COALESCE(source.execution_policy->'profiles','[]')) p WHERE p->>'runtime_id'=r.id::text))
 AND (r.visibility='public' OR r.owner_id=a.owner_id OR r.owner_id IS NULL OR a.owner_id IS NULL))
$$;

CREATE FUNCTION initialize_task_execution() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE a agent%ROWTYPE; previous agent_task_queue%ROWTYPE;
BEGIN
 SELECT * INTO a FROM agent WHERE id=NEW.agent_id;
 IF COALESCE(NEW.execution_selection,'{}')='{}' AND COALESCE(jsonb_array_length(execution_policy_for_agent(NEW.agent_id)->'profiles'),0)>0 THEN
  NEW.execution_selection=jsonb_build_object('state','pending');
 END IF;
 IF NEW.retry_of_task_id IS NOT NULL THEN
  SELECT * INTO previous FROM agent_task_queue WHERE id=NEW.retry_of_task_id;
  NEW.execution_request=previous.execution_request;
  IF previous.started_at IS NOT NULL THEN
   NEW.execution_selection=CASE WHEN previous.execution_selection->>'state'='selected' THEN previous.execution_selection || '{"locked":true}'::jsonb ELSE previous.execution_selection END;
   NEW.runtime_id=previous.runtime_id;
  END IF;
 END IF;
 RETURN NEW;
END $$;
CREATE TRIGGER task_execution_initialize BEFORE INSERT ON agent_task_queue FOR EACH ROW EXECUTE FUNCTION initialize_task_execution();

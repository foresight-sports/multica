CREATE OR REPLACE FUNCTION task_jira_allowed(iid uuid, aid uuid, ctx jsonb) RETURNS boolean LANGUAGE sql STABLE AS $$
  SELECT NOT EXISTS (
    SELECT 1 FROM issue_property p
    WHERE p.workspace_id=COALESCE((SELECT workspace_id FROM issue WHERE id=iid),(SELECT workspace_id FROM agent WHERE id=aid))
      AND p.config->>'system_key'='jira_ticket_id' AND p.config->>'required'='true'
      AND ((iid IS NOT NULL AND COALESCE(issue_jira_key(iid),'') !~ '^[A-Z][A-Z0-9_]*-[1-9][0-9]*$')
        OR (iid IS NULL AND ctx->>'type'='quick_create'))
  )
$$;
CREATE OR REPLACE FUNCTION enforce_jira_task_start() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.status IN ('dispatched','running') AND NEW.status IS DISTINCT FROM OLD.status
    AND NOT task_jira_allowed(NEW.issue_id,NEW.agent_id,NEW.context) THEN
    RAISE EXCEPTION 'JIRA ticket ID is required before agent work can start';
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER jira_task_start BEFORE UPDATE OF status ON agent_task_queue
FOR EACH ROW EXECUTE FUNCTION enforce_jira_task_start();

CREATE OR REPLACE FUNCTION maintain_issue_jira_key() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE pid text; inherited text; value text;
BEGIN
  SELECT id::text INTO pid FROM issue_property WHERE workspace_id=NEW.workspace_id AND config->>'system_key'='jira_ticket_id' LIMIT 1;
  IF pid IS NULL THEN RETURN NEW; END IF;
  IF TG_OP='INSERT' AND NEW.parent_issue_id IS NOT NULL AND NOT NEW.properties ? pid THEN
    SELECT issue_jira_key(id) INTO inherited FROM issue WHERE id=NEW.parent_issue_id AND workspace_id=NEW.workspace_id;
    IF COALESCE(inherited,'')<>'' THEN NEW.properties=jsonb_set(NEW.properties,ARRAY[pid],to_jsonb(inherited)); END IF;
  END IF;
  IF NEW.properties ? pid AND NEW.properties->pid <> 'null'::jsonb THEN
    value=NEW.properties->>pid;
    IF jsonb_typeof(NEW.properties->pid)<>'string' OR length(value)>128 OR value !~ '^[A-Z][A-Z0-9_]*-[1-9][0-9]*$' THEN
      RAISE EXCEPTION 'JIRA ticket ID must be a key such as FS-123';
    END IF;
  END IF;
  IF TG_OP='UPDATE' AND NEW.properties->pid IS DISTINCT FROM OLD.properties->pid
    AND EXISTS(SELECT 1 FROM agent_task_queue WHERE issue_id=NEW.id AND status IN ('dispatched','running','waiting_local_directory')) THEN
    RAISE EXCEPTION 'Stop the active run before changing its JIRA ticket ID';
  END IF;
  RETURN NEW;
END $$;

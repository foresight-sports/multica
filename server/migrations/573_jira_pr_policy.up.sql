-- Keep the compatibility function for older servers; tracking never gates runs.
DROP TRIGGER IF EXISTS jira_task_start ON agent_task_queue;
CREATE OR REPLACE FUNCTION task_jira_allowed(iid uuid, aid uuid, ctx jsonb) RETURNS boolean LANGUAGE sql STABLE AS $$ SELECT true $$;
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
  IF TG_OP='UPDATE' AND COALESCE(OLD.properties->>pid,'')<>'' AND NEW.properties->pid IS DISTINCT FROM OLD.properties->pid
    AND EXISTS(SELECT 1 FROM agent_task_queue WHERE issue_id=NEW.id AND status IN ('dispatched','running','waiting_local_directory')) THEN
    RAISE EXCEPTION 'Stop the active run before changing its JIRA ticket ID';
  END IF;
  RETURN NEW;
END $$;

-- Retry only runs blocked specifically by the retired Jira startup policy.
UPDATE agent_task_queue SET execution_selection=jsonb_build_object('state','pending','history',COALESCE(execution_selection->'history','[]'::jsonb))
WHERE status='queued' AND execution_selection->>'state'='blocked'
AND execution_selection->>'reason' LIKE '%JIRA ticket ID is required before agent work can start%';

-- Intake is a workspace-wide coordinator. Keep its roster complete across
-- agent creation/binding and intake configuration, without changing invocation grants.
CREATE OR REPLACE FUNCTION sync_intake_roster(ws uuid) RETURNS void
LANGUAGE plpgsql AS $$
BEGIN
  -- Serialize configuration and new agent commits for the same workspace.
  PERFORM id FROM workspace WHERE id = ws FOR UPDATE;
  INSERT INTO squad_member (squad_id, member_type, member_id, role)
  SELECT s.id, 'agent', a.id, ''
  FROM workspace w
  JOIN squad s ON s.workspace_id = w.id AND s.archived_at IS NULL
  JOIN agent a ON a.workspace_id = w.id AND a.archived_at IS NULL AND a.kind = 'user'
  WHERE w.id = ws AND (
    s.id::text = w.issue_intake->>'default_squad_id'
    OR EXISTS (SELECT 1 FROM jsonb_each_text(COALESCE(NULLIF(w.issue_intake->'projects', 'null'::jsonb), '{}'::jsonb)) p WHERE p.value = s.id::text)
  )
  ON CONFLICT (squad_id, member_type, member_id) DO NOTHING;
END;
$$;

CREATE OR REPLACE FUNCTION sync_intake_roster_trigger() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  IF TG_TABLE_NAME = 'workspace' THEN
    PERFORM sync_intake_roster(NEW.id);
  ELSE
    PERFORM sync_intake_roster(NEW.workspace_id);
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER intake_roster_workspace AFTER INSERT OR UPDATE OF issue_intake ON workspace
FOR EACH ROW EXECUTE FUNCTION sync_intake_roster_trigger();
CREATE TRIGGER intake_roster_agent AFTER INSERT OR UPDATE OF archived_at, workspace_id ON agent
FOR EACH ROW EXECUTE FUNCTION sync_intake_roster_trigger();
CREATE TRIGGER intake_roster_squad AFTER INSERT OR UPDATE OF archived_at ON squad
FOR EACH ROW EXECUTE FUNCTION sync_intake_roster_trigger();

SELECT sync_intake_roster(id) FROM workspace;

-- Review readiness is a handoff, not closure. Preserve the closure trigger.
ALTER TABLE issue_child_event DROP CONSTRAINT IF EXISTS issue_child_event_kind_check;
ALTER TABLE issue_child_event ADD CONSTRAINT issue_child_event_kind_check CHECK (kind IN ('closed','reopened','attached','detached','restaged','review_ready'));
CREATE OR REPLACE FUNCTION record_child_review_ready() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 INSERT INTO issue_child_event(workspace_id,parent_id,child_id,kind,source_task_id)
 VALUES(NEW.workspace_id,NEW.parent_issue_id,NEW.id,'review_ready',NULLIF(current_setting('multica.source_task_id',true),'')::uuid);
 RETURN NULL;
END $$;
CREATE TRIGGER child_review_ready_insert AFTER INSERT ON issue FOR EACH ROW
 WHEN (NEW.parent_issue_id IS NOT NULL AND NEW.status='in_review') EXECUTE FUNCTION record_child_review_ready();
CREATE TRIGGER child_review_ready_update AFTER UPDATE OF status,parent_issue_id ON issue FOR EACH ROW
 WHEN (NEW.parent_issue_id IS NOT NULL AND NEW.status='in_review' AND
 (OLD.status IS DISTINCT FROM NEW.status OR OLD.parent_issue_id IS DISTINCT FROM NEW.parent_issue_id))
 EXECUTE FUNCTION record_child_review_ready();

DROP TRIGGER IF EXISTS child_review_ready_insert ON issue;
DROP TRIGGER IF EXISTS child_review_ready_update ON issue;
DROP FUNCTION IF EXISTS record_child_review_ready();
DELETE FROM issue_child_event WHERE kind='review_ready';
ALTER TABLE issue_child_event DROP CONSTRAINT IF EXISTS issue_child_event_kind_check;
ALTER TABLE issue_child_event ADD CONSTRAINT issue_child_event_kind_check CHECK (kind IN ('closed','reopened','attached','detached','restaged'));

INSERT INTO inbox_item (id, workspace_id, recipient_type, recipient_id, type, severity, issue_id, title, body, actor_type, actor_id, details)
SELECT w.id, w.workspace_id, 'member', w.machine_owner_id,
 CASE WHEN w.state='pending' THEN 'installation_approval' ELSE 'installation_decided' END,
 CASE WHEN w.state='pending' THEN 'action_required' ELSE 'info' END,
 w.issue_id, i.title, concat(w.data->>'tool', ' ', w.data->>'version', ' · ', w.data->>'machine'), 'agent', w.agent_id,
 jsonb_build_object('work_record_id', w.id::text, 'state', w.state)
FROM work_record w JOIN issue i ON i.id=w.issue_id AND i.workspace_id=w.workspace_id
WHERE w.kind='installation' AND w.state='pending' AND w.machine_owner_id IS NOT NULL
ON CONFLICT (id) DO NOTHING;

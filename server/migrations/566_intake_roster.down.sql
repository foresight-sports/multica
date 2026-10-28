DROP TRIGGER IF EXISTS intake_roster_workspace ON workspace;
DROP TRIGGER IF EXISTS intake_roster_agent ON agent;
DROP TRIGGER IF EXISTS intake_roster_squad ON squad;
DROP FUNCTION IF EXISTS sync_intake_roster_trigger();
DROP FUNCTION IF EXISTS sync_intake_roster(uuid);
-- Keep memberships: removing them could discard subsequent human decisions.

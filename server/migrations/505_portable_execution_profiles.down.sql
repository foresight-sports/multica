-- Portable profiles cannot be losslessly rebound to one machine. Restore a
-- database backup when rolling back this migration; never invent bindings.
DO $$ BEGIN RAISE EXCEPTION 'Portable execution profiles require a database backup to roll back'; END $$;

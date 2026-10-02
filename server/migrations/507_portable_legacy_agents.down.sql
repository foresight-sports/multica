DO $$ BEGIN RAISE EXCEPTION 'Portable agents cannot be rebound losslessly. Restore the pre-upgrade backup to roll back.'; END $$;

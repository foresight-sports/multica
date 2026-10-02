DO $$ BEGIN RAISE EXCEPTION 'Portable execution admission requires a database backup to roll back'; END $$;

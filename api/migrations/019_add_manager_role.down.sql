-- Revert manager role addition
DO $$
DECLARE
  c_name TEXT;
BEGIN
  SELECT conname INTO c_name
  FROM pg_constraint
  WHERE conrelid = 'users'::regclass AND contype = 'c';
  IF c_name IS NOT NULL THEN
    EXECUTE 'ALTER TABLE users DROP CONSTRAINT ' || quote_ident(c_name);
  END IF;
END $$;

ALTER TABLE users
  ADD CONSTRAINT users_role_check CHECK (role IN ('admin','dev','support'));


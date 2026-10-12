-- Atlas local development — extensions.
-- Runs once, on first initialisation of an empty data volume.
-- (To re-run init scripts: `task nuke` then `task up`.)

CREATE EXTENSION IF NOT EXISTS citext;     -- case-insensitive text (emails, names)
CREATE EXTENSION IF NOT EXISTS pgcrypto;   -- digest / gen_random_uuid helpers
CREATE EXTENSION IF NOT EXISTS pg_trgm;    -- trigram search (duplicate detection, §8.1)

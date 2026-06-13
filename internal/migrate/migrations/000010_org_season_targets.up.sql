ALTER TABLE organizations
  ADD COLUMN IF NOT EXISTS season_targets jsonb NOT NULL DEFAULT '{}'::jsonb;

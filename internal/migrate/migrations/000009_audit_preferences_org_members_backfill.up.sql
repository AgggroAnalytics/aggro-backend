-- Field change audit (name, description, seasons; geometry when added later)
CREATE TABLE IF NOT EXISTS field_audit_log (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  field_id UUID NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  action TEXT NOT NULL,
  payload JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_field_audit_field_created
  ON field_audit_log(field_id, created_at DESC);

-- Per-user UI preferences (profile extras + app defaults)
CREATE TABLE IF NOT EXISTS user_preferences (
  user_id UUID PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
  locale TEXT NOT NULL DEFAULT 'ru',
  timezone TEXT NOT NULL DEFAULT 'UTC',
  avatar_url TEXT,
  units_system TEXT NOT NULL DEFAULT 'metric',
  date_format TEXT NOT NULL DEFAULT 'dmy',
  fields_default_year INT,
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Ensure org creators appear in organization_members (RBAC + UI list)
INSERT INTO organization_members (organization_id, user_id, role, created_at)
SELECT o.id, o.created_by, 'admin'::user_role, o.created_at
FROM organizations o
ON CONFLICT (organization_id, user_id) DO NOTHING;

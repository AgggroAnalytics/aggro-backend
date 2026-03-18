CREATE TYPE user_role AS ENUM(
  'admin',
  'manager',
  'farmer',
  'viewer'
);
CREATE TABLE IF NOT EXISTS users(
  id UUID PRIMARY KEY,
  username TEXT NOT NULL UNIQUE,
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL,
  email TEXT NOT NULL UNIQUE
);
CREATE TABLE IF NOT EXISTS organizations(
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  created_by UUID NOT NULL REFERENCES users(id)
ON DELETE RESTRICT
);
CREATE TABLE IF NOT EXISTS organization_members(
  organization_id UUID NOT NULL REFERENCES organizations(id)
ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id)
ON DELETE CASCADE,
  role user_role NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(organization_id,
  user_id)
);
CREATE INDEX IF NOT EXISTS idx_fields_organization_id
ON fields(organization_id);
CREATE INDEX IF NOT EXISTS idx_organizations_created_by
ON organizations(created_by);
CREATE INDEX IF NOT EXISTS idx_organization_members_user_id
ON organization_members(user_id);

-- name: ListOrganizationsForUser :many
SELECT o.id, o.name
FROM organizations o
WHERE o.created_by = sqlc.arg(user_id)
   OR EXISTS (
     SELECT 1 FROM organization_members m
     WHERE m.organization_id = o.id AND m.user_id = sqlc.arg(user_id)
   )
ORDER BY o.name;

-- name: CreateOrganization :one 
INSERT INTO organizations(
  id,
  name,
  created_at,
  created_by
) VALUES($1,$2,$3,$4) RETURNING id;

-- name: InviteMemberToOrganization :exec 
INSERT INTO organization_members(
  organization_id,
  user_id,
  role,
  created_at
) VALUES($1,$2,$3,$4);

-- name: UpsertOrganizationMember :exec
INSERT INTO organization_members (organization_id, user_id, role, created_at)
VALUES ($1, $2, $3, $4)
ON CONFLICT (organization_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: ListOrganizationMembers :many
SELECT
  u.id AS user_id,
  u.username,
  u.email,
  u.first_name,
  u.last_name,
  m.role,
  m.created_at AS member_since
FROM organization_members m
JOIN users u ON u.id = m.user_id
WHERE m.organization_id = $1
ORDER BY u.username;

-- name: GetOrganizationCreatedBy :one
SELECT created_by FROM organizations WHERE id = $1;

-- name: GetOrganizationMemberRole :one
SELECT role FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: UpdateOrganizationMemberRole :exec
UPDATE organization_members SET role = $3
WHERE organization_id = $1 AND user_id = $2;

-- name: RemoveOrganizationMember :exec
DELETE FROM organization_members
WHERE organization_id = $1 AND user_id = $2;

-- name: GetOrganizationSeasonTargets :one
SELECT season_targets FROM organizations WHERE id = sqlc.arg(id);

-- name: UpdateOrganizationSeasonTargets :exec
UPDATE organizations
SET season_targets = sqlc.arg(season_targets)::jsonb
WHERE id = sqlc.arg(id);

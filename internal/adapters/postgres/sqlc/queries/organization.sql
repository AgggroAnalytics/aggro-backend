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

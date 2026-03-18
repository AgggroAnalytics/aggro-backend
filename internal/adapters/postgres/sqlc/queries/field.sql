-- name: CreateField :one
INSERT INTO fields (
  id,
  name,
  description,
  geometry,
  organization_id
)
VALUES (
  sqlc.arg(id),
  sqlc.arg(name),
  sqlc.arg(description),
  ST_GeomFromWKB(sqlc.arg(geometry_wkb)::bytea, 4326),
  sqlc.arg(organization_id)
)
RETURNING id;

-- name: GetFieldByID :one
SELECT id, name, description, created_at, ST_AsBinary(geometry)::bytea AS geometry_wkb, area_hectares, organization_id
FROM fields
WHERE id = sqlc.arg(id);

-- name: ListFieldsByOrganizationID :many
SELECT id, name, description, created_at, ST_AsBinary(geometry)::bytea AS geometry_wkb, area_hectares, organization_id
FROM fields
WHERE organization_id = sqlc.arg(organization_id)
ORDER BY created_at DESC;

-- name: UpdateField :exec
UPDATE fields
SET name = sqlc.arg(name), description = sqlc.arg(description)
WHERE id = sqlc.arg(id);

-- name: DeleteField :exec
DELETE FROM fields WHERE id = sqlc.arg(id);

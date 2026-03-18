-- name: CreateTile :one
INSERT INTO tiles (id, field_id, geometry)
VALUES (
  gen_random_uuid(),
  sqlc.arg(field_id),
  ST_GeomFromWKB(sqlc.arg(geometry_wkb)::bytea, 4326)
)
RETURNING id;

-- name: ListTilesByFieldID :many
SELECT id, field_id, geometry
FROM tiles
WHERE field_id = sqlc.arg(field_id)
ORDER BY id;

-- name: ListTileIDsByFieldID :many
SELECT id
FROM tiles
WHERE field_id = sqlc.arg(field_id)
ORDER BY id;

-- name: ListTilesGeoJSONByFieldID :many
SELECT id, ST_AsGeoJSON(geometry)::json AS geometry_json
FROM tiles
WHERE field_id = sqlc.arg(field_id)
ORDER BY id;

-- name: DeleteTilesByFieldID :exec
DELETE FROM tiles
WHERE field_id = sqlc.arg(field_id);

-- name: InsertTileWithID :exec
INSERT INTO tiles (id, field_id, geometry)
VALUES (
  sqlc.arg(id),
  sqlc.arg(field_id),
  ST_GeomFromWKB(sqlc.arg(geometry_wkb)::bytea, 4326)
);

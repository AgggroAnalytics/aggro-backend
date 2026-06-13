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
SELECT
  f.id,
  f.name,
  f.description,
  f.created_at,
  ST_AsBinary(f.geometry)::bytea AS geometry_wkb,
  f.area_hectares,
  f.organization_id,
  COALESCE((SELECT COUNT(*) FROM tiles t WHERE t.field_id = f.id), 0)::int AS tile_count,
  COALESCE((SELECT COUNT(*) FROM seasons s WHERE s.field_id = f.id), 0)::int AS season_count,
  (SELECT MAX(tt.observation_date) FROM tile_timeseries tt
    INNER JOIN tiles t ON t.id = tt.tile_id AND t.field_id = f.id)::timestamptz AS latest_observation_at,
  COALESCE((
    SELECT COUNT(DISTINCT fa.observation_date)
    FROM field_analytics_timeseries fa
    WHERE fa.field_id = f.id AND fa.source = 'observed'::analytics_source
  ), 0)::int AS observed_analytics_dates,
  COALESCE((SELECT COUNT(*) FROM analysis_pmtiles_artifacts apa WHERE apa.field_id = f.id), 0)::int AS pmtiles_layer_count
FROM fields f
WHERE f.organization_id = sqlc.arg(organization_id)
ORDER BY f.created_at DESC;

-- name: UpdateField :exec
UPDATE fields
SET name = sqlc.arg(name), description = sqlc.arg(description)
WHERE id = sqlc.arg(id);

-- name: DeleteField :exec
DELETE FROM fields WHERE id = sqlc.arg(id);

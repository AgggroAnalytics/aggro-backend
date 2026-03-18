-- name: GetMaxObservationDateForField :one
SELECT MAX(tt.observation_date)::timestamptz AS max_date
FROM tile_timeseries tt
JOIN tiles t ON t.id = tt.tile_id
WHERE t.field_id = sqlc.arg(field_id);

-- name: ListTileTimeseriesByTileID :many
SELECT id, tile_id, observation_date,
  vh, vv, nbr2, ndmi, ndre, ndvi, gndvi, msavi,
  dry_days, bare_soil_index, valid_pixel_ratio,
  temperature_c_mean, precipitation_mm_3d, precipitation_mm_7d, precipitation_mm_30d,
  stress_index, created_at
FROM tile_timeseries
WHERE tile_id = sqlc.arg(tile_id)
ORDER BY observation_date DESC
LIMIT 50;

-- name: InsertTileTimeseries :one
INSERT INTO tile_timeseries (
  id, tile_id, observation_date,
  vh, vv, nbr2, ndmi, ndre, ndvi, gndvi, msavi,
  dry_days, bare_soil_index, valid_pixel_ratio,
  temperature_c_mean, precipitation_mm_3d, precipitation_mm_7d, precipitation_mm_30d
)
VALUES (
  gen_random_uuid(),
  sqlc.arg(tile_id),
  sqlc.arg(observation_date),
  sqlc.narg(vh), sqlc.narg(vv), sqlc.narg(nbr2), sqlc.narg(ndmi), sqlc.narg(ndre),
  sqlc.narg(ndvi), sqlc.narg(gndvi), sqlc.narg(msavi),
  sqlc.narg(dry_days), sqlc.narg(bare_soil_index), sqlc.narg(valid_pixel_ratio),
  sqlc.narg(temperature_c_mean), sqlc.narg(precipitation_mm_3d),
  sqlc.narg(precipitation_mm_7d), sqlc.narg(precipitation_mm_30d)
)
ON CONFLICT (tile_id, observation_date) DO UPDATE SET
  vh = EXCLUDED.vh, vv = EXCLUDED.vv, nbr2 = EXCLUDED.nbr2,
  ndmi = EXCLUDED.ndmi, ndre = EXCLUDED.ndre, ndvi = EXCLUDED.ndvi,
  gndvi = EXCLUDED.gndvi, msavi = EXCLUDED.msavi,
  dry_days = EXCLUDED.dry_days, bare_soil_index = EXCLUDED.bare_soil_index,
  valid_pixel_ratio = EXCLUDED.valid_pixel_ratio,
  temperature_c_mean = EXCLUDED.temperature_c_mean,
  precipitation_mm_3d = EXCLUDED.precipitation_mm_3d,
  precipitation_mm_7d = EXCLUDED.precipitation_mm_7d,
  precipitation_mm_30d = EXCLUDED.precipitation_mm_30d
RETURNING id;

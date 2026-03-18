-- name: ListFieldAnalyticsByFieldID :many
SELECT id, field_id, observation_date, source,
  tile_count, valid_tile_count,
  vh_mean, vv_mean, nbr2_mean, ndmi_mean, ndre_mean, ndvi_mean, gndvi_mean, msavi_mean,
  dry_days_mean, bare_soil_index_mean, valid_pixel_ratio_mean,
  temperature_c_mean, precipitation_mm_3d_mean, precipitation_mm_7d_mean, precipitation_mm_30d_mean,
  stress_index_mean, ndvi_stddev, ndmi_stddev, ndre_stddev,
  created_at
FROM field_analytics_timeseries
WHERE field_id = sqlc.arg(field_id)
  AND (sqlc.narg(date_from)::timestamptz IS NULL OR observation_date >= sqlc.narg(date_from))
  AND (sqlc.narg(date_to)::timestamptz IS NULL OR observation_date <= sqlc.narg(date_to))
ORDER BY observation_date DESC;

-- Upsert aggregated field metrics for one (field_id, observation_date) from tile_timeseries.
-- name: UpsertFieldAnalyticsForFieldAndDate :exec
INSERT INTO field_analytics_timeseries (
  id, field_id, observation_date, source,
  tile_count, valid_tile_count,
  vh_mean, vv_mean, nbr2_mean, ndmi_mean, ndre_mean, ndvi_mean, gndvi_mean, msavi_mean,
  dry_days_mean, bare_soil_index_mean, valid_pixel_ratio_mean,
  temperature_c_mean, precipitation_mm_3d_mean, precipitation_mm_7d_mean, precipitation_mm_30d_mean,
  stress_index_mean, ndvi_stddev, ndmi_stddev, ndre_stddev,
  created_at
)
SELECT
  gen_random_uuid(),
  t.field_id,
  tt.observation_date,
  'observed'::analytics_source,
  COUNT(*)::int4,
  COUNT(*) FILTER (WHERE tt.valid_pixel_ratio IS NOT NULL AND tt.valid_pixel_ratio > 0)::int4,
  AVG(tt.vh), AVG(tt.vv), AVG(tt.nbr2), AVG(tt.ndmi), AVG(tt.ndre), AVG(tt.ndvi), AVG(tt.gndvi), AVG(tt.msavi),
  AVG(tt.dry_days), AVG(tt.bare_soil_index), AVG(tt.valid_pixel_ratio),
  AVG(tt.temperature_c_mean), AVG(tt.precipitation_mm_3d), AVG(tt.precipitation_mm_7d), AVG(tt.precipitation_mm_30d),
  AVG(tt.stress_index),
  STDDEV(tt.ndvi), STDDEV(tt.ndmi), STDDEV(tt.ndre),
  now()
FROM tile_timeseries tt
JOIN tiles t ON t.id = tt.tile_id
WHERE t.field_id = sqlc.arg(field_id)
  AND tt.observation_date = sqlc.arg(observation_date)
GROUP BY t.field_id, tt.observation_date
ON CONFLICT (field_id, observation_date, source) DO UPDATE SET
  tile_count = EXCLUDED.tile_count,
  valid_tile_count = EXCLUDED.valid_tile_count,
  vh_mean = EXCLUDED.vh_mean,
  vv_mean = EXCLUDED.vv_mean,
  nbr2_mean = EXCLUDED.nbr2_mean,
  ndmi_mean = EXCLUDED.ndmi_mean,
  ndre_mean = EXCLUDED.ndre_mean,
  ndvi_mean = EXCLUDED.ndvi_mean,
  gndvi_mean = EXCLUDED.gndvi_mean,
  msavi_mean = EXCLUDED.msavi_mean,
  dry_days_mean = EXCLUDED.dry_days_mean,
  bare_soil_index_mean = EXCLUDED.bare_soil_index_mean,
  valid_pixel_ratio_mean = EXCLUDED.valid_pixel_ratio_mean,
  temperature_c_mean = EXCLUDED.temperature_c_mean,
  precipitation_mm_3d_mean = EXCLUDED.precipitation_mm_3d_mean,
  precipitation_mm_7d_mean = EXCLUDED.precipitation_mm_7d_mean,
  precipitation_mm_30d_mean = EXCLUDED.precipitation_mm_30d_mean,
  stress_index_mean = EXCLUDED.stress_index_mean,
  ndvi_stddev = EXCLUDED.ndvi_stddev,
  ndmi_stddev = EXCLUDED.ndmi_stddev,
  ndre_stddev = EXCLUDED.ndre_stddev,
  created_at = now();

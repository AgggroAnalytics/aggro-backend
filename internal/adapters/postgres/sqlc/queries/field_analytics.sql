-- name: ListFieldAnalyticsByFieldID :many
SELECT id, field_id, observation_date, source,
  tile_count, valid_tile_count,
  vh_mean, vv_mean, nbr2_mean, ndmi_mean, ndre_mean, ndvi_mean, gndvi_mean, msavi_mean,
  dry_days_mean, bare_soil_index_mean, valid_pixel_ratio_mean,
  temperature_c_mean, precipitation_mm_3d_mean, precipitation_mm_7d_mean, precipitation_mm_30d_mean,
  stress_index_mean, ndvi_stddev, ndmi_stddev, ndre_stddev,
  heterogeneity_score,
  prediction_degradation_score, prediction_vegetation_cover_loss_score, prediction_bare_soil_expansion_score,
  prediction_health_score, prediction_stress_score_total, prediction_water_stress, prediction_confidence,
  prediction_under_irrigation_risk_score, prediction_over_irrigation_risk_score, prediction_uniformity_score,
  prediction_vegetation_activity_drop, prediction_heterogeneity_growth, prediction_irrigation_events_detected,
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

-- name: UpsertFieldPredictedAnalyticsForFieldAndDate :exec
INSERT INTO field_analytics_timeseries (
  id, field_id, observation_date, source,
  tile_count,
  heterogeneity_score,
  prediction_degradation_score, prediction_vegetation_cover_loss_score, prediction_bare_soil_expansion_score,
  prediction_health_score, prediction_stress_score_total, prediction_water_stress, prediction_confidence,
  prediction_under_irrigation_risk_score, prediction_over_irrigation_risk_score, prediction_uniformity_score,
  prediction_vegetation_activity_drop, prediction_heterogeneity_growth, prediction_irrigation_events_detected,
  created_at
) VALUES (
  gen_random_uuid(),
  sqlc.arg(field_id),
  sqlc.arg(observation_date),
  'predicted'::analytics_source,
  sqlc.arg(tile_count),
  sqlc.narg(heterogeneity_score),
  sqlc.narg(prediction_degradation_score),
  sqlc.narg(prediction_vegetation_cover_loss_score),
  sqlc.narg(prediction_bare_soil_expansion_score),
  sqlc.narg(prediction_health_score),
  sqlc.narg(prediction_stress_score_total),
  sqlc.narg(prediction_water_stress),
  sqlc.narg(prediction_confidence),
  sqlc.narg(prediction_under_irrigation_risk_score),
  sqlc.narg(prediction_over_irrigation_risk_score),
  sqlc.narg(prediction_uniformity_score),
  sqlc.narg(prediction_vegetation_activity_drop),
  sqlc.narg(prediction_heterogeneity_growth),
  sqlc.narg(prediction_irrigation_events_detected),
  now()
)
ON CONFLICT (field_id, observation_date, source) DO UPDATE SET
  tile_count = EXCLUDED.tile_count,
  heterogeneity_score = EXCLUDED.heterogeneity_score,
  prediction_degradation_score = EXCLUDED.prediction_degradation_score,
  prediction_vegetation_cover_loss_score = EXCLUDED.prediction_vegetation_cover_loss_score,
  prediction_bare_soil_expansion_score = EXCLUDED.prediction_bare_soil_expansion_score,
  prediction_health_score = EXCLUDED.prediction_health_score,
  prediction_stress_score_total = EXCLUDED.prediction_stress_score_total,
  prediction_water_stress = EXCLUDED.prediction_water_stress,
  prediction_confidence = EXCLUDED.prediction_confidence,
  prediction_under_irrigation_risk_score = EXCLUDED.prediction_under_irrigation_risk_score,
  prediction_over_irrigation_risk_score = EXCLUDED.prediction_over_irrigation_risk_score,
  prediction_uniformity_score = EXCLUDED.prediction_uniformity_score,
  prediction_vegetation_activity_drop = EXCLUDED.prediction_vegetation_activity_drop,
  prediction_heterogeneity_growth = EXCLUDED.prediction_heterogeneity_growth,
  prediction_irrigation_events_detected = EXCLUDED.prediction_irrigation_events_detected,
  created_at = now();

-- name: DeletePredictedFieldAnalyticsByFieldID :exec
DELETE FROM field_analytics_timeseries
WHERE field_id = sqlc.arg(field_id) AND source = 'predicted';

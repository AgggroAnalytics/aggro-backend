-- name: ListTilePredictionsByTileID :many
SELECT id, tile_id, module, prediction_date, status, model_version, created_at
FROM tile_predictions
WHERE tile_id = sqlc.arg(tile_id)
ORDER BY prediction_date DESC
LIMIT 50;

-- name: GetTilePredictionDegradation :one
SELECT id, degradation_score, degradation_level, trend,
  vegetation_cover_loss_score, bare_soil_expansion_score, heterogeneity_score,
  alert_level, explanations
FROM tile_predictions_degradation
WHERE id = sqlc.arg(id);

-- name: GetTilePredictionHealthStress :one
SELECT id, health_score, stress_score_total, water_stress,
  vegetation_activity_drop, heterogeneity_growth, alert_level, trend, explanations
FROM tile_predictions_health_stress
WHERE id = sqlc.arg(id);

-- name: GetTilePredictionIrrigation :one
SELECT id, is_irrigated, confidence, water_balance_status,
  under_irrigation_risk_score, over_irrigation_risk_score, uniformity_score, explanations
FROM tile_predictions_irrigation_water_use
WHERE id = sqlc.arg(id);

-- name: InsertTilePrediction :one
INSERT INTO tile_predictions (id, tile_id, module, prediction_date, status)
VALUES (gen_random_uuid(), $1, $2, $3, 'completed')
ON CONFLICT (tile_id, module, prediction_date) DO UPDATE SET status = 'completed'
RETURNING id;

-- name: InsertTilePredictionDegradation :exec
INSERT INTO tile_predictions_degradation (
  id, degradation_score, degradation_level, trend,
  vegetation_cover_loss_score, bare_soil_expansion_score, heterogeneity_score,
  alert_level, explanations
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
  degradation_score = EXCLUDED.degradation_score,
  degradation_level = EXCLUDED.degradation_level,
  trend = EXCLUDED.trend,
  vegetation_cover_loss_score = EXCLUDED.vegetation_cover_loss_score,
  bare_soil_expansion_score = EXCLUDED.bare_soil_expansion_score,
  heterogeneity_score = EXCLUDED.heterogeneity_score,
  alert_level = EXCLUDED.alert_level,
  explanations = EXCLUDED.explanations;

-- name: InsertTilePredictionHealthStress :exec
INSERT INTO tile_predictions_health_stress (
  id, health_score, stress_score_total,
  water_stress, vegetation_activity_drop, heterogeneity_growth,
  alert_level, trend, explanations
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
ON CONFLICT (id) DO UPDATE SET
  health_score = EXCLUDED.health_score,
  stress_score_total = EXCLUDED.stress_score_total,
  water_stress = EXCLUDED.water_stress,
  vegetation_activity_drop = EXCLUDED.vegetation_activity_drop,
  heterogeneity_growth = EXCLUDED.heterogeneity_growth,
  alert_level = EXCLUDED.alert_level,
  trend = EXCLUDED.trend,
  explanations = EXCLUDED.explanations;

-- name: InsertTilePredictionIrrigation :exec
INSERT INTO tile_predictions_irrigation_water_use (
  id, is_irrigated, confidence, water_balance_status,
  under_irrigation_risk_score, over_irrigation_risk_score, uniformity_score,
  explanations
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (id) DO UPDATE SET
  is_irrigated = EXCLUDED.is_irrigated,
  confidence = EXCLUDED.confidence,
  water_balance_status = EXCLUDED.water_balance_status,
  under_irrigation_risk_score = EXCLUDED.under_irrigation_risk_score,
  over_irrigation_risk_score = EXCLUDED.over_irrigation_risk_score,
  uniformity_score = EXCLUDED.uniformity_score,
  explanations = EXCLUDED.explanations;

-- Field-level means for ML / predicted metrics (one row per date, source = predicted).
ALTER TABLE field_analytics_timeseries
  ADD COLUMN IF NOT EXISTS prediction_degradation_score NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_vegetation_cover_loss_score NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_bare_soil_expansion_score NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_health_score NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_stress_score_total NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_water_stress NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_confidence NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_under_irrigation_risk_score NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_over_irrigation_risk_score NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_uniformity_score NUMERIC;

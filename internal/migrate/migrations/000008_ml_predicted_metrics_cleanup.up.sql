-- ML field-level aggregates aligned with worker JSON (m0/m1/m2).
ALTER TABLE field_analytics_timeseries
  ADD COLUMN IF NOT EXISTS prediction_vegetation_activity_drop NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_heterogeneity_growth NUMERIC,
  ADD COLUMN IF NOT EXISTS prediction_irrigation_events_detected NUMERIC;

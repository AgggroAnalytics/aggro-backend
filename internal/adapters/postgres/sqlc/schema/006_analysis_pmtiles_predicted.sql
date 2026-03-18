-- Sync with migration 000006 (for sqlc schema completeness on fresh DBs).
ALTER TABLE analysis_pmtiles_artifacts
  DROP CONSTRAINT IF EXISTS analysis_pmtiles_artifacts_module_check;

ALTER TABLE analysis_pmtiles_artifacts
  ADD CONSTRAINT analysis_pmtiles_artifacts_module_check
  CHECK (module IN ('', 'degradation', 'health_stress', 'irrigation_water_use', 'predicted'));

-- Allow combined predicted PMTiles URL per date from Temporal workflow finalizer.
ALTER TABLE analysis_pmtiles_artifacts
  DROP CONSTRAINT IF EXISTS analysis_pmtiles_artifacts_module_check;

ALTER TABLE analysis_pmtiles_artifacts
  ADD CONSTRAINT analysis_pmtiles_artifacts_module_check
  CHECK (module IN ('', 'degradation', 'health_stress', 'irrigation_water_use', 'predicted'));

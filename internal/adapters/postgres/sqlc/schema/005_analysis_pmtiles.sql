-- One PMTiles bundle per analysis result (observed per date, or prediction per module per date).
-- module = '' for observed, 'degradation'|'health_stress'|'irrigation_water_use' for prediction.
CREATE TABLE IF NOT EXISTS analysis_pmtiles_artifacts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  field_id UUID NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  analysis_kind TEXT NOT NULL CHECK (analysis_kind IN ('observed', 'prediction')),
  analysis_date DATE NOT NULL,
  module TEXT NOT NULL DEFAULT '' CHECK (module IN ('', 'degradation', 'health_stress', 'irrigation_water_use')),
  pmtiles_url TEXT NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT analysis_pmtiles_artifacts_unique UNIQUE (field_id, analysis_kind, analysis_date, module)
);

CREATE INDEX IF NOT EXISTS idx_analysis_pmtiles_artifacts_field_id
  ON analysis_pmtiles_artifacts (field_id, analysis_date DESC);

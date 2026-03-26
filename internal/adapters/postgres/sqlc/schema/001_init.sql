CREATE EXTENSION IF NOT EXISTS postgis;

-- =========================================================
-- ENUMS
-- =========================================================


CREATE TYPE prediction_module AS ENUM (
  'health_stress',
  'irrigation_water_use',
  'degradation'
);

CREATE TYPE prediction_status AS ENUM (
  'in_progress',
  'completed',
  'failed'
);

CREATE TYPE alert_level AS ENUM (
  'none',
  'low',
  'medium',
  'high',
  'critical'
);

CREATE TYPE trend_direction AS ENUM (
  'improving',
  'stable',
  'worsening'
);

CREATE TYPE water_balance_status AS ENUM (
  'balanced',
  'under_irrigated',
  'over_irrigated',
  'unknown'
);

CREATE TYPE degradation_level AS ENUM (
  'none',
  'low',
  'medium',
  'high',
  'severe'
);

-- Optional: field analytics / aggregation source
CREATE TYPE analytics_source AS ENUM (
  'observed',
  'derived',
  'predicted'
);

-- =========================================================
-- CORE TABLES
-- =========================================================


CREATE TABLE IF NOT EXISTS fields (
  id UUID PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT,
  created_at timestamptz NOT NULL DEFAULT now(),
  geometry geometry(Polygon, 4326) NOT NULL,
  area_hectares numeric GENERATED ALWAYS AS (
  ST_Area(geometry::geography) / 10000
  ) STORED,
  organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  CONSTRAINT fields_area_hectares_nonnegative CHECK (area_hectares IS NULL OR area_hectares >= 0)
);


CREATE TABLE IF NOT EXISTS tiles (
  id UUID PRIMARY KEY,
  field_id UUID NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  geometry geometry(Polygon, 4326) NOT NULL
);

-- =========================================================
-- TILE OBSERVATIONS / TIMESERIES
-- =========================================================

CREATE TABLE IF NOT EXISTS tile_timeseries (
  id UUID PRIMARY KEY,
  tile_id UUID NOT NULL REFERENCES tiles(id) ON DELETE CASCADE,
  observation_date timestamptz NOT NULL,

  vh NUMERIC,
  vv NUMERIC,

  nbr2 NUMERIC,
  ndmi NUMERIC,
  ndre NUMERIC,
  ndvi NUMERIC,
  gndvi NUMERIC,
  msavi NUMERIC,

  dry_days INTEGER,
  bare_soil_index NUMERIC,
  valid_pixel_ratio NUMERIC,

  temperature_c_mean NUMERIC,
  precipitation_mm_3d NUMERIC,
  precipitation_mm_7d NUMERIC,
  precipitation_mm_30d NUMERIC,

  stress_index NUMERIC,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT tile_timeseries_unique_tile_date UNIQUE (tile_id, observation_date),
  CONSTRAINT tile_timeseries_dry_days_nonnegative CHECK (dry_days IS NULL OR dry_days >= 0),
  CONSTRAINT tile_timeseries_valid_pixel_ratio_range CHECK (
    valid_pixel_ratio IS NULL OR (valid_pixel_ratio >= 0 AND valid_pixel_ratio <= 1)
  )
);

-- =========================================================
-- TILE PREDICTIONS
-- =========================================================

CREATE TABLE IF NOT EXISTS tile_predictions (
  id UUID PRIMARY KEY,
  tile_id UUID NOT NULL REFERENCES tiles(id) ON DELETE CASCADE,
  module prediction_module NOT NULL,
  prediction_date timestamptz NOT NULL,
  status prediction_status NOT NULL DEFAULT 'in_progress',
  model_version TEXT,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT tile_predictions_unique UNIQUE (tile_id, module, prediction_date)
);

CREATE TABLE IF NOT EXISTS tile_predictions_health_stress (
  id UUID PRIMARY KEY REFERENCES tile_predictions(id) ON DELETE CASCADE,
  health_score NUMERIC,
  stress_score_total NUMERIC,
  water_stress NUMERIC,
  vegetation_activity_drop NUMERIC,
  heterogeneity_growth NUMERIC,
  alert_level alert_level,
  trend trend_direction,
  explanations jsonb NOT NULL DEFAULT '[]'::jsonb
);

CREATE TABLE IF NOT EXISTS tile_predictions_irrigation_water_use (
  id UUID PRIMARY KEY REFERENCES tile_predictions(id) ON DELETE CASCADE,
  is_irrigated BOOLEAN,
  confidence NUMERIC,
  water_balance_status water_balance_status,
  under_irrigation_risk_score NUMERIC,
  over_irrigation_risk_score NUMERIC,
  uniformity_score NUMERIC,
  explanations jsonb NOT NULL DEFAULT '[]'::jsonb,

  CONSTRAINT tile_pred_irrigation_confidence_range CHECK (
    confidence IS NULL OR (confidence >= 0 AND confidence <= 1)
  )
);

CREATE TABLE IF NOT EXISTS tile_predictions_degradation (
  id UUID PRIMARY KEY REFERENCES tile_predictions(id) ON DELETE CASCADE,
  degradation_score NUMERIC,
  degradation_level degradation_level,
  trend trend_direction,
  vegetation_cover_loss_score NUMERIC,
  bare_soil_expansion_score NUMERIC,
  heterogeneity_score NUMERIC,
  alert_level alert_level,
  explanations jsonb NOT NULL DEFAULT '[]'::jsonb
);

-- =========================================================
-- FIELD ANALYTICS (AGGREGATED OBSERVED / DERIVED METRICS)
-- =========================================================

CREATE TABLE IF NOT EXISTS field_analytics_timeseries (
  id UUID PRIMARY KEY,
  field_id UUID NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  observation_date timestamptz NOT NULL,
  source analytics_source NOT NULL DEFAULT 'observed',

  tile_count INTEGER,
  valid_tile_count INTEGER,

  vh_mean NUMERIC,
  vv_mean NUMERIC,

  nbr2_mean NUMERIC,
  ndmi_mean NUMERIC,
  ndre_mean NUMERIC,
  ndvi_mean NUMERIC,
  gndvi_mean NUMERIC,
  msavi_mean NUMERIC,

  dry_days_mean NUMERIC,
  bare_soil_index_mean NUMERIC,
  valid_pixel_ratio_mean NUMERIC,

  temperature_c_mean NUMERIC,
  precipitation_mm_3d_mean NUMERIC,
  precipitation_mm_7d_mean NUMERIC,
  precipitation_mm_30d_mean NUMERIC,

  stress_index_mean NUMERIC,
  stress_index_p25 NUMERIC,
  stress_index_p50 NUMERIC,
  stress_index_p75 NUMERIC,
  stress_index_stddev NUMERIC,

  ndvi_stddev NUMERIC,
  ndmi_stddev NUMERIC,
  ndre_stddev NUMERIC,
  heterogeneity_score NUMERIC,

  prediction_degradation_score NUMERIC,
  prediction_vegetation_cover_loss_score NUMERIC,
  prediction_bare_soil_expansion_score NUMERIC,
  prediction_health_score NUMERIC,
  prediction_stress_score_total NUMERIC,
  prediction_water_stress NUMERIC,
  prediction_confidence NUMERIC,
  prediction_under_irrigation_risk_score NUMERIC,
  prediction_over_irrigation_risk_score NUMERIC,
  prediction_uniformity_score NUMERIC,
  prediction_vegetation_activity_drop NUMERIC,
  prediction_heterogeneity_growth NUMERIC,
  prediction_irrigation_events_detected NUMERIC,

  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT field_analytics_unique UNIQUE (field_id, observation_date, source),
  CONSTRAINT field_analytics_tile_count_nonnegative CHECK (
    tile_count IS NULL OR tile_count >= 0
  ),
  CONSTRAINT field_analytics_valid_tile_count_nonnegative CHECK (
    valid_tile_count IS NULL OR valid_tile_count >= 0
  ),
  CONSTRAINT field_analytics_valid_pixel_ratio_mean_range CHECK (
    valid_pixel_ratio_mean IS NULL OR (valid_pixel_ratio_mean >= 0 AND valid_pixel_ratio_mean <= 1)
  )
);

-- =========================================================
-- FIELD PREDICTIONS
-- =========================================================

CREATE TABLE IF NOT EXISTS field_predictions (
  id UUID PRIMARY KEY,
  field_id UUID NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  module prediction_module NOT NULL,
  prediction_date timestamptz NOT NULL,
  status prediction_status NOT NULL DEFAULT 'in_progress',
  model_version TEXT,
  created_at timestamptz NOT NULL DEFAULT now(),

  CONSTRAINT field_predictions_unique UNIQUE (field_id, module, prediction_date)
);

CREATE TABLE IF NOT EXISTS field_predictions_health_stress (
  id UUID PRIMARY KEY REFERENCES field_predictions(id) ON DELETE CASCADE,
  health_score NUMERIC,
  stress_score_total NUMERIC,
  water_stress NUMERIC,
  vegetation_activity_drop NUMERIC,
  heterogeneity_growth NUMERIC,
  alert_level alert_level,
  trend trend_direction,
  affected_tile_ratio NUMERIC,
  explanations jsonb NOT NULL DEFAULT '[]'::jsonb,

  CONSTRAINT field_pred_health_affected_tile_ratio_range CHECK (
    affected_tile_ratio IS NULL OR (affected_tile_ratio >= 0 AND affected_tile_ratio <= 1)
  )
);

CREATE TABLE IF NOT EXISTS field_predictions_irrigation_water_use (
  id UUID PRIMARY KEY REFERENCES field_predictions(id) ON DELETE CASCADE,
  is_irrigated BOOLEAN,
  confidence NUMERIC,
  water_balance_status water_balance_status,
  under_irrigation_risk_score NUMERIC,
  over_irrigation_risk_score NUMERIC,
  uniformity_score NUMERIC,
  affected_tile_ratio NUMERIC,
  explanations jsonb NOT NULL DEFAULT '[]'::jsonb,

  CONSTRAINT field_pred_irrigation_confidence_range CHECK (
    confidence IS NULL OR (confidence >= 0 AND confidence <= 1)
  ),
  CONSTRAINT field_pred_irrigation_affected_tile_ratio_range CHECK (
    affected_tile_ratio IS NULL OR (affected_tile_ratio >= 0 AND affected_tile_ratio <= 1)
  )
);

CREATE TABLE IF NOT EXISTS field_predictions_degradation (
  id UUID PRIMARY KEY REFERENCES field_predictions(id) ON DELETE CASCADE,
  degradation_score NUMERIC,
  degradation_level degradation_level,
  trend trend_direction,
  vegetation_cover_loss_score NUMERIC,
  bare_soil_expansion_score NUMERIC,
  heterogeneity_score NUMERIC,
  alert_level alert_level,
  affected_tile_ratio NUMERIC,
  explanations jsonb NOT NULL DEFAULT '[]'::jsonb,

  CONSTRAINT field_pred_degradation_affected_tile_ratio_range CHECK (
    affected_tile_ratio IS NULL OR (affected_tile_ratio >= 0 AND affected_tile_ratio <= 1)
  )
);

-- =========================================================
-- INDEXES
-- =========================================================


CREATE INDEX IF NOT EXISTS idx_fields_geometry
  ON fields USING GIST (geometry);

CREATE INDEX IF NOT EXISTS idx_tiles_field_id
  ON tiles (field_id);

CREATE INDEX IF NOT EXISTS idx_tiles_geometry
  ON tiles USING GIST (geometry);

CREATE INDEX IF NOT EXISTS idx_tile_timeseries_tile_id_observation_date
  ON tile_timeseries (tile_id, observation_date DESC);

CREATE INDEX IF NOT EXISTS idx_tile_predictions_tile_id_prediction_date
  ON tile_predictions (tile_id, prediction_date DESC);

CREATE INDEX IF NOT EXISTS idx_field_analytics_field_id_observation_date
  ON field_analytics_timeseries (field_id, observation_date DESC);

CREATE INDEX IF NOT EXISTS idx_field_predictions_field_id_prediction_date
  ON field_predictions (field_id, prediction_date DESC);

CREATE TABLE IF NOT EXISTS seasons (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  field_id UUID NOT NULL REFERENCES fields(id) ON DELETE CASCADE,
  name TEXT NOT NULL,
  start_date DATE NOT NULL,
  end_date DATE NOT NULL,
  is_auto BOOLEAN NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT seasons_dates_order CHECK (start_date <= end_date),
  CONSTRAINT seasons_max_3_months CHECK (
    is_auto = true OR (end_date - start_date <= 93)
  )
);

CREATE INDEX IF NOT EXISTS idx_seasons_field_id ON seasons(field_id);

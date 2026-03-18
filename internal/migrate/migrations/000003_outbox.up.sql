CREATE TABLE IF NOT EXISTS outbox(
  id uuid PRIMARY KEY,
  aggregate_type text,
  aggregate_id uuid,
  message_type text,
  message_payload jsonb NOT NULL DEFAULT '[]'::jsonb,
  created_at timestamptz NOT NULL DEFAULT now(),
  retry_count integer NOT NULL DEFAULT 0,
  next_attempt_at timestamptz,
  processed_at timestamptz NOT NULL DEFAULT now(),
  status text
);

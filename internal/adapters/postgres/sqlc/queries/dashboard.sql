-- name: OrgDashboardStats :one
WITH x AS (SELECT sqlc.arg(organization_id)::uuid AS org_id)
SELECT
  (SELECT COUNT(*)::int FROM fields fi WHERE fi.organization_id = (SELECT org_id FROM x)) AS field_count,
  (SELECT COALESCE(SUM(fi.area_hectares), 0)::float8 FROM fields fi WHERE fi.organization_id = (SELECT org_id FROM x)) AS total_area_ha,
  (SELECT COUNT(DISTINCT fa.field_id)::int
   FROM field_analytics_timeseries fa
   INNER JOIN fields fi ON fi.id = fa.field_id AND fi.organization_id = (SELECT org_id FROM x)
   WHERE fa.source = 'observed'::analytics_source) AS fields_with_observed_analytics,
  (SELECT COUNT(*)::int FROM organization_members om WHERE om.organization_id = (SELECT org_id FROM x)) AS member_count;

-- name: OrgObservedNdviWeekly :many
SELECT
  date_trunc('week', fa.observation_date AT TIME ZONE 'UTC')::timestamptz AS week_start,
  AVG(fa.ndvi_mean)::float8 AS ndvi_mean_avg
FROM field_analytics_timeseries fa
INNER JOIN fields f ON f.id = fa.field_id AND f.organization_id = sqlc.arg(organization_id)
WHERE fa.source = 'observed'::analytics_source
  AND fa.ndvi_mean IS NOT NULL
  AND fa.observation_date >= (now() AT TIME ZONE 'UTC') - interval '8 weeks'
GROUP BY 1
ORDER BY 1 ASC;

-- name: OrgFieldsStaleProcessing :many
SELECT
  f.id,
  f.name,
  (
    SELECT MAX(fa.observation_date)::timestamptz
    FROM field_analytics_timeseries fa
    WHERE fa.field_id = f.id AND fa.source = 'observed'::analytics_source
  ) AS last_analytics_at,
  COALESCE((SELECT COUNT(*) FROM tiles t WHERE t.field_id = f.id), 0)::int AS tile_count
FROM fields f
WHERE f.organization_id = sqlc.arg(organization_id)
  AND COALESCE((SELECT COUNT(*) FROM tiles t WHERE t.field_id = f.id), 0) > 0
  AND (
    NOT EXISTS (
      SELECT 1 FROM field_analytics_timeseries fa
      WHERE fa.field_id = f.id AND fa.source = 'observed'::analytics_source
    )
    OR (
      SELECT MAX(fa.observation_date)
      FROM field_analytics_timeseries fa
      WHERE fa.field_id = f.id AND fa.source = 'observed'::analytics_source
    ) < (now() AT TIME ZONE 'UTC') - interval '30 days'
  )
ORDER BY f.name
LIMIT sqlc.arg(row_limit);

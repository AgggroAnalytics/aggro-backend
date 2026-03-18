-- name: ListAnalysisPmtilesByFieldID :many
SELECT id, field_id, analysis_kind, analysis_date, module, pmtiles_url, created_at
FROM analysis_pmtiles_artifacts
WHERE field_id = sqlc.arg(field_id)
ORDER BY analysis_date DESC;

-- name: UpsertAnalysisPmtilesArtifact :exec
INSERT INTO analysis_pmtiles_artifacts (field_id, analysis_kind, analysis_date, module, pmtiles_url)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (field_id, analysis_kind, analysis_date, module)
DO UPDATE SET pmtiles_url = EXCLUDED.pmtiles_url, created_at = now();

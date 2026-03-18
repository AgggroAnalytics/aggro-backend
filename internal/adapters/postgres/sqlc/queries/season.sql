-- name: CreateSeason :one
INSERT INTO seasons (id, field_id, name, start_date, end_date, is_auto)
VALUES (
  gen_random_uuid(),
  sqlc.arg(field_id),
  sqlc.arg(name),
  sqlc.arg(start_date),
  sqlc.arg(end_date),
  sqlc.arg(is_auto)
)
RETURNING id;

-- name: GetSeasonByID :one
SELECT id, field_id, name, start_date, end_date, is_auto, created_at
FROM seasons
WHERE id = sqlc.arg(id);

-- name: ListSeasonsByFieldID :many
SELECT id, field_id, name, start_date, end_date, is_auto, created_at
FROM seasons
WHERE field_id = sqlc.arg(field_id)
ORDER BY start_date DESC;

-- name: UpdateSeason :exec
UPDATE seasons
SET name = sqlc.arg(name), start_date = sqlc.arg(start_date), end_date = sqlc.arg(end_date), is_auto = sqlc.arg(is_auto)
WHERE id = sqlc.arg(id);

-- name: DeleteSeason :exec
DELETE FROM seasons WHERE id = sqlc.arg(id);

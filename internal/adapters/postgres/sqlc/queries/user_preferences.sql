-- name: GetUserPreferences :one
SELECT user_id, locale, timezone, avatar_url, units_system, date_format, fields_default_year, updated_at
FROM user_preferences
WHERE user_id = sqlc.arg(user_id);

-- name: UpsertUserPreferences :exec
INSERT INTO user_preferences (
  user_id, locale, timezone, avatar_url, units_system, date_format, fields_default_year, updated_at
) VALUES (
  sqlc.arg(user_id),
  sqlc.arg(locale),
  sqlc.arg(timezone),
  sqlc.arg(avatar_url),
  sqlc.arg(units_system),
  sqlc.arg(date_format),
  sqlc.arg(fields_default_year),
  now()
)
ON CONFLICT (user_id) DO UPDATE SET
  locale = EXCLUDED.locale,
  timezone = EXCLUDED.timezone,
  avatar_url = EXCLUDED.avatar_url,
  units_system = EXCLUDED.units_system,
  date_format = EXCLUDED.date_format,
  fields_default_year = EXCLUDED.fields_default_year,
  updated_at = now();

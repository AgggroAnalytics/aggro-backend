-- name: InsertFieldAuditLog :exec
INSERT INTO field_audit_log (id, field_id, actor_user_id, action, payload, created_at)
VALUES (gen_random_uuid(), sqlc.arg(field_id), sqlc.arg(actor_user_id), sqlc.arg(action), sqlc.arg(payload), now());

-- name: ListFieldAuditLogByFieldID :many
SELECT id, field_id, actor_user_id, action, payload, created_at
FROM field_audit_log
WHERE field_id = sqlc.arg(field_id)
ORDER BY created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListFieldAuditLogByOrganizationID :many
SELECT
  a.id,
  a.field_id,
  a.actor_user_id,
  a.action,
  a.payload,
  a.created_at,
  f.name AS field_name
FROM field_audit_log a
INNER JOIN fields f ON f.id = a.field_id AND f.organization_id = sqlc.arg(organization_id)
ORDER BY a.created_at DESC
LIMIT sqlc.arg(row_limit);

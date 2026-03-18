-- name: CreateOutboxEvent :exec
INSERT INTO outbox(id, aggregate_type, aggregate_id, message_type, message_payload, created_at, status)
VALUES($1,$2,$3,$4,$5,$6, $7);

-- name: ClaimOneEvent :one
UPDATE outbox
SET status = 'processing'
WHERE id = (
    SELECT id
    FROM outbox
    WHERE status = 'pending' AND next_attempt_at <= now() AND retry_count < 3
    ORDER BY created_at
    LIMIT 1
    FOR UPDATE SKIP LOCKED
)
RETURNING id, aggregate_type, aggregate_id, message_type, message_payload, created_at, status, retry_count;

-- name: MarkSent :exec
UPDATE outbox
SET
status = 'sent',
processed_at = now()
WHERE id = $1 AND status = 'processing';

-- name: MarkFailed :exec
UPDATE outbox
SET
status = 'pending',
next_attempt_at = now() + interval '30 seconds',
retry_count = retry_count + 1
WHERE id = $1 AND status = 'processing';

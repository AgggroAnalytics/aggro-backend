-- name: GetUserByEmail :one
SELECT id, username, first_name, last_name, email
FROM users
WHERE email = sqlc.arg(email);

-- name: GetUserByID :one
SELECT id, username, first_name, last_name, email
FROM users
WHERE id = sqlc.arg(id);

-- name: GetUserByUsername :one
SELECT id, username, first_name, last_name, email
FROM users
WHERE username = sqlc.arg(username);

-- name: CreateUser :exec
INSERT INTO users (id, username, first_name, last_name, email)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.arg(email));

-- name: UpsertUser :exec
INSERT INTO users (id, username, first_name, last_name, email)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.arg(email))
ON CONFLICT (id) DO UPDATE SET
  username = EXCLUDED.username,
  first_name = EXCLUDED.first_name,
  last_name = EXCLUDED.last_name,
  email = EXCLUDED.email;

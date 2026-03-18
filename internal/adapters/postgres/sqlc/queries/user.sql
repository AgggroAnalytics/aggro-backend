-- name: GetUserByEmail :one
SELECT id, username, first_name, last_name, email
FROM users
WHERE email = sqlc.arg(email);

-- name: GetUserByID :one
SELECT id, username, first_name, last_name, email
FROM users
WHERE id = sqlc.arg(id);

-- name: CreateUser :exec
INSERT INTO users (id, username, first_name, last_name, email)
VALUES (sqlc.arg(id), sqlc.arg(username), sqlc.arg(first_name), sqlc.arg(last_name), sqlc.arg(email));

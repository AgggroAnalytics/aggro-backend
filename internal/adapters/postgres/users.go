package postgres

import (
	"context"
	"errors"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UsersPostgres struct {
	pool *pgxpool.Pool
}

func NewUsersPostgres(pool *pgxpool.Pool) *UsersPostgres {
	return &UsersPostgres{pool: pool}
}

func (r *UsersPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *UsersPostgres) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	u, err := r.queries(ctx).GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return sqlcUserToDomain(u), nil
}

func (r *UsersPostgres) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	u, err := r.queries(ctx).GetUserByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return sqlcUserToDomain(u), nil
}

func (r *UsersPostgres) Create(ctx context.Context, user *domain.User) error {
	return r.queries(ctx).CreateUser(ctx, sqlc.CreateUserParams{
		ID:        user.ID,
		Username:  user.Username,
		FirstName: user.Firstname,
		LastName:  user.LastName,
		Email:     user.Email,
	})
}

func sqlcUserToDomain(u sqlc.User) *domain.User {
	return &domain.User{
		ID:        u.ID,
		Username:  u.Username,
		Firstname: u.FirstName,
		LastName:  u.LastName,
		Email:     u.Email,
	}
}

var _ ports.UserRepository = (*UsersPostgres)(nil)

package postgres

import (
	"context"
	"errors"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type UserPreferencesPostgres struct {
	pool *pgxpool.Pool
}

func NewUserPreferencesPostgres(pool *pgxpool.Pool) *UserPreferencesPostgres {
	return &UserPreferencesPostgres{pool: pool}
}

func (r *UserPreferencesPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *UserPreferencesPostgres) Get(ctx context.Context, userID uuid.UUID) (*ports.UserPreferences, error) {
	row, err := r.queries(ctx).GetUserPreferences(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &ports.UserPreferences{
				UserID:      userID,
				Locale:      "ru",
				Timezone:    "UTC",
				UnitsSystem: "metric",
				DateFormat:  "dmy",
			}, nil
		}
		return nil, err
	}
	p := &ports.UserPreferences{
		UserID:      row.UserID,
		Locale:      row.Locale,
		Timezone:    row.Timezone,
		UnitsSystem: row.UnitsSystem,
		DateFormat:  row.DateFormat,
	}
	if row.AvatarUrl.Valid {
		p.AvatarURL = row.AvatarUrl.String
	}
	if row.FieldsDefaultYear.Valid {
		y := row.FieldsDefaultYear.Int32
		p.FieldsDefaultYear = &y
	}
	if row.UpdatedAt.Valid {
		p.UpdatedAt = row.UpdatedAt.Time
	}
	return p, nil
}

func (r *UserPreferencesPostgres) Upsert(ctx context.Context, p *ports.UserPreferences) error {
	av := pgtype.Text{}
	if p.AvatarURL != "" {
		av = pgtype.Text{String: p.AvatarURL, Valid: true}
	}
	fy := pgtype.Int4{}
	if p.FieldsDefaultYear != nil {
		fy = pgtype.Int4{Int32: *p.FieldsDefaultYear, Valid: true}
	}
	return r.queries(ctx).UpsertUserPreferences(ctx, sqlc.UpsertUserPreferencesParams{
		UserID:            p.UserID,
		Locale:            p.Locale,
		Timezone:          p.Timezone,
		AvatarUrl:         av,
		UnitsSystem:       p.UnitsSystem,
		DateFormat:        p.DateFormat,
		FieldsDefaultYear: fy,
	})
}

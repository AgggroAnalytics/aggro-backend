package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SeasonsPostgres struct {
	pool *pgxpool.Pool
}

func NewSeasonsPostgres(pool *pgxpool.Pool) *SeasonsPostgres {
	return &SeasonsPostgres{pool: pool}
}

func (r *SeasonsPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *SeasonsPostgres) CreateSeason(ctx context.Context, fieldID uuid.UUID, name string, startDate, endDate time.Time, isAuto bool) (uuid.UUID, error) {
	q := r.queries(ctx)
	start := pgtype.Date{Time: startDate, Valid: true}
	end := pgtype.Date{Time: endDate, Valid: true}
	return q.CreateSeason(ctx, sqlc.CreateSeasonParams{
		FieldID:   fieldID,
		Name:      name,
		StartDate: start,
		EndDate:   end,
		IsAuto:    isAuto,
	})
}

func (r *SeasonsPostgres) ListSeasonsByFieldID(ctx context.Context, fieldID uuid.UUID) ([]domain.Season, error) {
	q := r.queries(ctx)
	list, err := q.ListSeasonsByFieldID(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Season, 0, len(list))
	for _, s := range list {
		var startTime, endTime time.Time
		if s.StartDate.Valid {
			startTime = s.StartDate.Time
		}
		if s.EndDate.Valid {
			endTime = s.EndDate.Time
		}
		out = append(out, domain.Season{
			ID:        s.ID,
			FieldID:   s.FieldID,
			Name:      s.Name,
			StartDate: startTime,
			EndDate:   endTime,
			IsAuto:    s.IsAuto,
		})
	}
	return out, nil
}

func (r *SeasonsPostgres) GetSeasonByID(ctx context.Context, id uuid.UUID) (*domain.Season, error) {
	q := r.queries(ctx)
	s, err := q.GetSeasonByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	var startTime, endTime time.Time
	if s.StartDate.Valid {
		startTime = s.StartDate.Time
	}
	if s.EndDate.Valid {
		endTime = s.EndDate.Time
	}
	return &domain.Season{
		ID:        s.ID,
		FieldID:   s.FieldID,
		Name:      s.Name,
		StartDate: startTime,
		EndDate:   endTime,
		IsAuto:    s.IsAuto,
	}, nil
}

func (r *SeasonsPostgres) UpdateSeason(ctx context.Context, id uuid.UUID, name string, startDate, endDate time.Time, isAuto bool) error {
	q := r.queries(ctx)
	return q.UpdateSeason(ctx, sqlc.UpdateSeasonParams{
		ID:        id,
		Name:      name,
		StartDate: pgtype.Date{Time: startDate, Valid: true},
		EndDate:   pgtype.Date{Time: endDate, Valid: true},
		IsAuto:    isAuto,
	})
}

func (r *SeasonsPostgres) DeleteSeason(ctx context.Context, id uuid.UUID) error {
	return r.queries(ctx).DeleteSeason(ctx, id)
}

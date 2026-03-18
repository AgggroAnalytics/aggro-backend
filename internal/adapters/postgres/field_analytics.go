package postgres

import (
	"context"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func pgFloat64(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, _ := n.Float64Value()
	return &f.Float64
}

func pgInt32(n pgtype.Int4) *int32 {
	if !n.Valid {
		return nil
	}
	return &n.Int32
}

type FieldAnalyticsPostgres struct {
	pool *pgxpool.Pool
}

func NewFieldAnalyticsPostgres(pool *pgxpool.Pool) *FieldAnalyticsPostgres {
	return &FieldAnalyticsPostgres{pool: pool}
}

func (r *FieldAnalyticsPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *FieldAnalyticsPostgres) UpsertFieldAnalyticsForFieldAndDate(ctx context.Context, fieldID uuid.UUID, observationDate time.Time) error {
	q := r.queries(ctx)
	return q.UpsertFieldAnalyticsForFieldAndDate(ctx, sqlc.UpsertFieldAnalyticsForFieldAndDateParams{
		FieldID:         fieldID,
		ObservationDate: pgtype.Timestamptz{Time: observationDate, Valid: true},
	})
}

func (r *FieldAnalyticsPostgres) ListFieldAnalyticsByFieldID(ctx context.Context, fieldID uuid.UUID, dateFrom, dateTo *time.Time) ([]ports.FieldAnalyticsRow, error) {
	q := r.queries(ctx)
	var from, to pgtype.Timestamptz
	if dateFrom != nil {
		from = pgtype.Timestamptz{Time: *dateFrom, Valid: true}
	}
	if dateTo != nil {
		to = pgtype.Timestamptz{Time: *dateTo, Valid: true}
	}
	list, err := q.ListFieldAnalyticsByFieldID(ctx, sqlc.ListFieldAnalyticsByFieldIDParams{
		FieldID:  fieldID,
		DateFrom: from,
		DateTo:   to,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.FieldAnalyticsRow, 0, len(list))
	for _, row := range list {
		var obsTime time.Time
		if row.ObservationDate.Valid {
			obsTime = row.ObservationDate.Time
		}
		var createdAt time.Time
		if row.CreatedAt.Valid {
			createdAt = row.CreatedAt.Time
		}
		out = append(out, ports.FieldAnalyticsRow{
			ID:                     row.ID,
			FieldID:                row.FieldID,
			ObservationDate:        obsTime,
			Source:                 string(row.Source),
			TileCount:              pgInt32(row.TileCount),
			ValidTileCount:         pgInt32(row.ValidTileCount),
			NdviMean:               pgFloat64(row.NdviMean),
			NdmiMean:               pgFloat64(row.NdmiMean),
			NdreMean:               pgFloat64(row.NdreMean),
			ValidPixelRatioMean:    pgFloat64(row.ValidPixelRatioMean),
			StressIndexMean:         pgFloat64(row.StressIndexMean),
			TemperatureCMean:       pgFloat64(row.TemperatureCMean),
			PrecipitationMm3dMean:   pgFloat64(row.PrecipitationMm3dMean),
			PrecipitationMm7dMean:   pgFloat64(row.PrecipitationMm7dMean),
			PrecipitationMm30dMean:  pgFloat64(row.PrecipitationMm30dMean),
			CreatedAt:              createdAt,
		})
	}
	return out, nil
}

var _ ports.FieldAnalyticsRepository = (*FieldAnalyticsPostgres)(nil)

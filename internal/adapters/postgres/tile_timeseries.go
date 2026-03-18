package postgres

import (
	"context"
	"strconv"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TileTimeseriesPostgres struct {
	pool *pgxpool.Pool
}

func NewTileTimeseriesPostgres(pool *pgxpool.Pool) *TileTimeseriesPostgres {
	return &TileTimeseriesPostgres{pool: pool}
}

func (r *TileTimeseriesPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *TileTimeseriesPostgres) GetMaxObservationDateForField(ctx context.Context, fieldID uuid.UUID) (*time.Time, error) {
	q := r.queries(ctx)
	t, err := q.GetMaxObservationDateForField(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	if !t.Valid {
		return nil, nil
	}
	tt := t.Time
	return &tt, nil
}

func floatPtrToNumeric(v *float64) pgtype.Numeric {
	if v == nil {
		return pgtype.Numeric{Valid: false}
	}
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(*v, 'f', -1, 64))
	return n
}

func intPtrToInt4(v *int32) pgtype.Int4 {
	if v == nil {
		return pgtype.Int4{Valid: false}
	}
	return pgtype.Int4{Int32: *v, Valid: true}
}

func (r *TileTimeseriesPostgres) InsertTileTimeseries(ctx context.Context, row *ports.TileTimeseriesRow) error {
	q := r.queries(ctx)
	obsDate := pgtype.Timestamptz{Time: row.ObservationDate, Valid: true}
	_, err := q.InsertTileTimeseries(ctx, sqlc.InsertTileTimeseriesParams{
		TileID:             row.TileID,
		ObservationDate:    obsDate,
		Vh:                 floatPtrToNumeric(row.Vh),
		Vv:                 floatPtrToNumeric(row.Vv),
		Nbr2:               floatPtrToNumeric(row.Nbr2),
		Ndmi:               floatPtrToNumeric(row.Ndmi),
		Ndre:               floatPtrToNumeric(row.Ndre),
		Ndvi:               floatPtrToNumeric(row.Ndvi),
		Gndvi:              floatPtrToNumeric(row.Gndvi),
		Msavi:              floatPtrToNumeric(row.Msavi),
		DryDays:            intPtrToInt4(row.DryDays),
		BareSoilIndex:      floatPtrToNumeric(row.BareSoilIndex),
		ValidPixelRatio:    floatPtrToNumeric(row.ValidPixelRatio),
		TemperatureCMean:   floatPtrToNumeric(row.TemperatureCMean),
		PrecipitationMm3d:  floatPtrToNumeric(row.PrecipitationMm3d),
		PrecipitationMm7d:  floatPtrToNumeric(row.PrecipitationMm7d),
		PrecipitationMm30d: floatPtrToNumeric(row.PrecipitationMm30d),
	})
	return err
}

func numericToFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, _ := n.Float64Value()
	if !f.Valid {
		return nil
	}
	v := f.Float64
	return &v
}

func int4ToInt32(n pgtype.Int4) *int32 {
	if !n.Valid {
		return nil
	}
	v := n.Int32
	return &v
}

func (r *TileTimeseriesPostgres) ListByTileID(ctx context.Context, tileID uuid.UUID) ([]ports.TileTimeseriesRow, error) {
	q := r.queries(ctx)
	rows, err := q.ListTileTimeseriesByTileID(ctx, tileID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.TileTimeseriesRow, 0, len(rows))
	for _, row := range rows {
		var obsDate time.Time
		if row.ObservationDate.Valid {
			obsDate = row.ObservationDate.Time
		}
		out = append(out, ports.TileTimeseriesRow{
			TileID:             row.TileID,
			ObservationDate:    obsDate,
			Vh:                 numericToFloat(row.Vh),
			Vv:                 numericToFloat(row.Vv),
			Nbr2:               numericToFloat(row.Nbr2),
			Ndmi:               numericToFloat(row.Ndmi),
			Ndre:               numericToFloat(row.Ndre),
			Ndvi:               numericToFloat(row.Ndvi),
			Gndvi:              numericToFloat(row.Gndvi),
			Msavi:              numericToFloat(row.Msavi),
			DryDays:            int4ToInt32(row.DryDays),
			BareSoilIndex:      numericToFloat(row.BareSoilIndex),
			ValidPixelRatio:    numericToFloat(row.ValidPixelRatio),
			TemperatureCMean:   numericToFloat(row.TemperatureCMean),
			PrecipitationMm3d:  numericToFloat(row.PrecipitationMm3d),
			PrecipitationMm7d:  numericToFloat(row.PrecipitationMm7d),
			PrecipitationMm30d: numericToFloat(row.PrecipitationMm30d),
		})
	}
	return out, nil
}

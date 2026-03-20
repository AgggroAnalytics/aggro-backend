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

func (r *FieldAnalyticsPostgres) DeleteFieldAnalyticsByDates(ctx context.Context, fieldID uuid.UUID, dates []time.Time) error {
	for _, d := range dates {
		day := d.UTC().Format("2006-01-02")
		if _, err := r.pool.Exec(
			ctx,
			`DELETE FROM field_analytics_timeseries
			 WHERE field_id = $1
			   AND (observation_date AT TIME ZONE 'UTC')::date = $2::date`,
			fieldID,
			day,
		); err != nil {
			return err
		}
	}
	return nil
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

func (r *FieldAnalyticsPostgres) UpsertFieldPredictedAnalyticsForFieldAndDate(ctx context.Context, fieldID uuid.UUID, observationDate time.Time, m ports.PredictedFieldAnalyticsMeans) error {
	q := r.queries(ctx)
	return q.UpsertFieldPredictedAnalyticsForFieldAndDate(ctx, sqlc.UpsertFieldPredictedAnalyticsForFieldAndDateParams{
		FieldID:                            fieldID,
		ObservationDate:                    pgtype.Timestamptz{Time: observationDate, Valid: true},
		TileCount:                          pgtype.Int4{Int32: m.TileCount, Valid: true},
		HeterogeneityScore:                 floatPtrToNumeric(m.HeterogeneityScore),
		PredictionDegradationScore:         floatPtrToNumeric(m.DegradationScore),
		PredictionVegetationCoverLossScore: floatPtrToNumeric(m.VegetationCoverLossScore),
		PredictionBareSoilExpansionScore:   floatPtrToNumeric(m.BareSoilExpansionScore),
		PredictionHealthScore:              floatPtrToNumeric(m.HealthScore),
		PredictionStressScoreTotal:         floatPtrToNumeric(m.StressScoreTotal),
		PredictionWaterStress:              floatPtrToNumeric(m.WaterStress),
		PredictionConfidence:               floatPtrToNumeric(m.Confidence),
		PredictionUnderIrrigationRiskScore: floatPtrToNumeric(m.UnderIrrigationRiskScore),
		PredictionOverIrrigationRiskScore:  floatPtrToNumeric(m.OverIrrigationRiskScore),
		PredictionUniformityScore:          floatPtrToNumeric(m.UniformityScore),
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
			ID:                                 row.ID,
			FieldID:                            row.FieldID,
			ObservationDate:                    obsTime,
			Source:                             string(row.Source),
			TileCount:                          pgInt32(row.TileCount),
			ValidTileCount:                     pgInt32(row.ValidTileCount),
			NdviMean:                           pgFloat64(row.NdviMean),
			NdmiMean:                           pgFloat64(row.NdmiMean),
			NdreMean:                           pgFloat64(row.NdreMean),
			GndviMean:                          pgFloat64(row.GndviMean),
			MsaviMean:                          pgFloat64(row.MsaviMean),
			Nbr2Mean:                           pgFloat64(row.Nbr2Mean),
			BareSoilIndexMean:                  pgFloat64(row.BareSoilIndexMean),
			ValidPixelRatioMean:                pgFloat64(row.ValidPixelRatioMean),
			StressIndexMean:                    pgFloat64(row.StressIndexMean),
			TemperatureCMean:                   pgFloat64(row.TemperatureCMean),
			PrecipitationMm3dMean:              pgFloat64(row.PrecipitationMm3dMean),
			PrecipitationMm7dMean:              pgFloat64(row.PrecipitationMm7dMean),
			PrecipitationMm30dMean:             pgFloat64(row.PrecipitationMm30dMean),
			HeterogeneityScore:                 pgFloat64(row.HeterogeneityScore),
			PredictionDegradationScore:         pgFloat64(row.PredictionDegradationScore),
			PredictionVegetationCoverLossScore: pgFloat64(row.PredictionVegetationCoverLossScore),
			PredictionBareSoilExpansionScore:   pgFloat64(row.PredictionBareSoilExpansionScore),
			PredictionHealthScore:              pgFloat64(row.PredictionHealthScore),
			PredictionStressScoreTotal:         pgFloat64(row.PredictionStressScoreTotal),
			PredictionWaterStress:              pgFloat64(row.PredictionWaterStress),
			PredictionConfidence:               pgFloat64(row.PredictionConfidence),
			PredictionUnderIrrigationRiskScore: pgFloat64(row.PredictionUnderIrrigationRiskScore),
			PredictionOverIrrigationRiskScore:  pgFloat64(row.PredictionOverIrrigationRiskScore),
			PredictionUniformityScore:          pgFloat64(row.PredictionUniformityScore),
			CreatedAt:                          createdAt,
		})
	}
	return out, nil
}

var _ ports.FieldAnalyticsRepository = (*FieldAnalyticsPostgres)(nil)

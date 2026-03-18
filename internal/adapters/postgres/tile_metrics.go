package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TileMetricsPostgres struct {
	pool *pgxpool.Pool
}

func NewTileMetricsPostgres(pool *pgxpool.Pool) *TileMetricsPostgres {
	return &TileMetricsPostgres{pool: pool}
}

func (r *TileMetricsPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *TileMetricsPostgres) GetTileMetrics(ctx context.Context, tileID uuid.UUID) (*ports.TileMetrics, error) {
	q := r.queries(ctx)

	observedList, err := q.ListTileTimeseriesByTileID(ctx, tileID)
	if err != nil {
		return nil, err
	}
	observed := make([]ports.TileObservedRow, 0, len(observedList))
	for _, row := range observedList {
		var obsTime time.Time
		if row.ObservationDate.Valid {
			obsTime = row.ObservationDate.Time
		}
		observed = append(observed, ports.TileObservedRow{
			ObservationDate:   obsTime,
			Ndvi:              pgNumToFloat(row.Ndvi),
			Ndmi:              pgNumToFloat(row.Ndmi),
			Ndre:              pgNumToFloat(row.Ndre),
			ValidPixelRatio:   pgNumToFloat(row.ValidPixelRatio),
			StressIndex:      pgNumToFloat(row.StressIndex),
			TemperatureCMean:  pgNumToFloat(row.TemperatureCMean),
			PrecipitationMm3d: pgNumToFloat(row.PrecipitationMm3d),
			Mm7d:             pgNumToFloat(row.PrecipitationMm7d),
			Mm30d:            pgNumToFloat(row.PrecipitationMm30d),
		})
	}

	predList, err := q.ListTilePredictionsByTileID(ctx, tileID)
	if err != nil {
		return nil, err
	}
	predictions := make([]ports.TilePredictionRow, 0, len(predList))
	for _, p := range predList {
		var predDate time.Time
		if p.PredictionDate.Valid {
			predDate = p.PredictionDate.Time
		}
		row := ports.TilePredictionRow{
			ID:             p.ID,
			Module:         string(p.Module),
			PredictionDate: predDate,
			Status:         string(p.Status),
		}
		switch p.Module {
		case sqlc.PredictionModuleDegradation:
			d, err := q.GetTilePredictionDegradation(ctx, p.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if err == nil {
				row.Degradation = &ports.TilePredictionDegradationDetail{
					DegradationScore:         pgNumToFloat(d.DegradationScore),
					DegradationLevel:         nullDegradationLevelString(d.DegradationLevel),
					Trend:                    nullTrendString(d.Trend),
					VegetationCoverLossScore: pgNumToFloat(d.VegetationCoverLossScore),
					BareSoilExpansionScore:   pgNumToFloat(d.BareSoilExpansionScore),
					HeterogeneityScore:       pgNumToFloat(d.HeterogeneityScore),
					AlertLevel:               nullAlertLevelString(d.AlertLevel),
				}
			}
		case sqlc.PredictionModuleHealthStress:
			h, err := q.GetTilePredictionHealthStress(ctx, p.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if err == nil {
				row.HealthStress = &ports.TilePredictionHealthStressDetail{
					HealthScore:            pgNumToFloat(h.HealthScore),
					StressScoreTotal:       pgNumToFloat(h.StressScoreTotal),
					WaterStress:            pgNumToFloat(h.WaterStress),
					VegetationActivityDrop: pgNumToFloat(h.VegetationActivityDrop),
					HeterogeneityGrowth:    pgNumToFloat(h.HeterogeneityGrowth),
					AlertLevel:             nullAlertLevelString(h.AlertLevel),
					Trend:                  nullTrendString(h.Trend),
				}
			}
		case sqlc.PredictionModuleIrrigationWaterUse:
			i, err := q.GetTilePredictionIrrigation(ctx, p.ID)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if err == nil {
				var irr *bool
				if i.IsIrrigated.Valid {
					irr = &i.IsIrrigated.Bool
				}
				row.Irrigation = &ports.TilePredictionIrrigationDetail{
					IsIrrigated:              irr,
					Confidence:               pgNumToFloat(i.Confidence),
					WaterBalanceStatus:       nullWaterBalanceString(i.WaterBalanceStatus),
					UnderIrrigationRiskScore: pgNumToFloat(i.UnderIrrigationRiskScore),
					OverIrrigationRiskScore:  pgNumToFloat(i.OverIrrigationRiskScore),
					UniformityScore:          pgNumToFloat(i.UniformityScore),
				}
			}
		}
		predictions = append(predictions, row)
	}

	return &ports.TileMetrics{
		TileID:     tileID,
		Observed:   observed,
		Predictions: predictions,
	}, nil
}

func pgNumToFloat(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	f, _ := n.Float64Value()
	return &f.Float64
}

func nullDegradationLevelString(n sqlc.NullDegradationLevel) string {
	if !n.Valid {
		return ""
	}
	return string(n.DegradationLevel)
}

func nullTrendString(n sqlc.NullTrendDirection) string {
	if !n.Valid {
		return ""
	}
	return string(n.TrendDirection)
}

func nullAlertLevelString(n sqlc.NullAlertLevel) string {
	if !n.Valid {
		return ""
	}
	return string(n.AlertLevel)
}

func nullWaterBalanceString(n sqlc.NullWaterBalanceStatus) string {
	if !n.Valid {
		return ""
	}
	return string(n.WaterBalanceStatus)
}

var _ ports.TileMetricsReader = (*TileMetricsPostgres)(nil)

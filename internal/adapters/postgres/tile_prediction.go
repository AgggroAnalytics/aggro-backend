package postgres

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type TilePredictionPostgres struct {
	pool *pgxpool.Pool
}

func NewTilePredictionPostgres(pool *pgxpool.Pool) *TilePredictionPostgres {
	return &TilePredictionPostgres{pool: pool}
}

func (r *TilePredictionPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *TilePredictionPostgres) SaveMLCompletion(ctx context.Context, body []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return err
	}
	module, _ := raw["module"]
	modStr := string(module)
	modStr = trimQuotes(modStr)
	tileIDRaw, ok := raw["tile_id"]
	if !ok {
		return nil
	}
	tileIDStr := trimQuotes(string(tileIDRaw))
	tileID, err := uuid.Parse(tileIDStr)
	if err != nil {
		return err
	}

	q := r.queries(ctx)
	predDate := pgtype.Timestamptz{Time: time.Now().UTC().Truncate(24 * time.Hour), Valid: true}
	var modEnum sqlc.PredictionModule
	switch modStr {
	case "m0":
		modEnum = sqlc.PredictionModuleDegradation
	case "m1":
		modEnum = sqlc.PredictionModuleHealthStress
	case "m2":
		modEnum = sqlc.PredictionModuleIrrigationWaterUse
	default:
		return nil
	}

	mainID, err := q.InsertTilePrediction(ctx, sqlc.InsertTilePredictionParams{
		TileID:         tileID,
		Module:         modEnum,
		PredictionDate: predDate,
	})
	if err != nil {
		return err
	}

	switch modStr {
	case "m0":
		return r.saveDegradation(ctx, q, mainID, body)
	case "m1":
		return r.saveHealthStress(ctx, q, mainID, body)
	case "m2":
		return r.saveIrrigation(ctx, q, mainID, body)
	}
	return nil
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

func floatToNumeric(v float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.FormatFloat(v, 'f', -1, 64))
	return n
}

func intToNumeric(v int) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(strconv.Itoa(v))
	return n
}

func jsonStringsToExplanations(explanations []string) []byte {
	if len(explanations) == 0 {
		return []byte("[]")
	}
	b, _ := json.Marshal(explanations)
	return b
}

func (r *TilePredictionPostgres) saveDegradation(ctx context.Context, q *sqlc.Queries, id uuid.UUID, body []byte) error {
	var p struct {
		DegradationScore         *int     `json:"degradation_score"`
		DegradationLevel         string   `json:"degradation_level"`
		DegradationClass         string   `json:"degradation_class"`
		Trend                    string   `json:"trend"`
		VegetationCoverLossScore *int     `json:"vegetation_cover_loss_score"`
		BareSoilExpansionScore   *int     `json:"bare_soil_expansion_score"`
		HeterogeneityScore       *int     `json:"heterogeneity_score"`
		AlertLevel               string   `json:"alert_level"`
		Explanations             []string `json:"explanations"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	if p.DegradationLevel == "" {
		p.DegradationLevel = p.DegradationClass
	}
	level := mapDegradationLevel(p.DegradationLevel)
	trend := mapTrend(p.Trend)
	alert := mapAlertLevel(p.AlertLevel)
	var degScore, vegScore, bareScore, hetScore pgtype.Numeric
	if p.DegradationScore != nil {
		degScore = intToNumeric(*p.DegradationScore)
	}
	if p.VegetationCoverLossScore != nil {
		vegScore = intToNumeric(*p.VegetationCoverLossScore)
	}
	if p.BareSoilExpansionScore != nil {
		bareScore = intToNumeric(*p.BareSoilExpansionScore)
	}
	if p.HeterogeneityScore != nil {
		hetScore = intToNumeric(*p.HeterogeneityScore)
	}
	return q.InsertTilePredictionDegradation(ctx, sqlc.InsertTilePredictionDegradationParams{
		ID:                       id,
		DegradationScore:         degScore,
		DegradationLevel:         level,
		Trend:                    trend,
		VegetationCoverLossScore: vegScore,
		BareSoilExpansionScore:   bareScore,
		HeterogeneityScore:       hetScore,
		AlertLevel:               alert,
		Explanations:             jsonStringsToExplanations(p.Explanations),
	})
}

func (r *TilePredictionPostgres) saveHealthStress(ctx context.Context, q *sqlc.Queries, id uuid.UUID, body []byte) error {
	var p struct {
		HealthScore      *int `json:"health_score"`
		StressScoreTotal *int `json:"stress_score_total"`
		StressScores     *struct {
			WaterStress            *int `json:"water_stress"`
			VegetationActivityDrop *int `json:"vegetation_activity_drop"`
			HeterogeneityGrowth    *int `json:"heterogeneity_growth"`
		} `json:"stress_scores"`
		AlertLevel   string   `json:"alert_level"`
		Trend        string   `json:"trend"`
		Explanations []string `json:"explanations"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	var health, stressTotal, water, vegDrop, hetGrowth pgtype.Numeric
	if p.HealthScore != nil {
		health = intToNumeric(*p.HealthScore)
	}
	if p.StressScoreTotal != nil {
		stressTotal = intToNumeric(*p.StressScoreTotal)
	}
	if p.StressScores != nil {
		if p.StressScores.WaterStress != nil {
			water = intToNumeric(*p.StressScores.WaterStress)
		}
		if p.StressScores.VegetationActivityDrop != nil {
			vegDrop = intToNumeric(*p.StressScores.VegetationActivityDrop)
		}
		if p.StressScores.HeterogeneityGrowth != nil {
			hetGrowth = intToNumeric(*p.StressScores.HeterogeneityGrowth)
		}
	}
	return q.InsertTilePredictionHealthStress(ctx, sqlc.InsertTilePredictionHealthStressParams{
		ID:                     id,
		HealthScore:            health,
		StressScoreTotal:       stressTotal,
		WaterStress:            water,
		VegetationActivityDrop: vegDrop,
		HeterogeneityGrowth:    hetGrowth,
		AlertLevel:             mapAlertLevel(p.AlertLevel),
		Trend:                  mapTrend(p.Trend),
		Explanations:           jsonStringsToExplanations(p.Explanations),
	})
}

func (r *TilePredictionPostgres) saveIrrigation(ctx context.Context, q *sqlc.Queries, id uuid.UUID, body []byte) error {
	var p struct {
		IsIrrigated              *bool    `json:"is_irrigated"`
		Confidence               *float64 `json:"confidence"`
		WaterBalanceStatus       string   `json:"water_balance_status"`
		IrrigationStatus         string   `json:"irrigation_status"`
		WaterBalanceRisk         string   `json:"water_balance_risk"`
		UnderIrrigationRiskScore *int     `json:"under_irrigation_risk_score"`
		OverIrrigationRiskScore  *int     `json:"over_irrigation_risk_score"`
		UniformityScore          *int     `json:"uniformity_score"`
		Explanations             []string `json:"explanations"`
	}
	if err := json.Unmarshal(body, &p); err != nil {
		return err
	}
	if p.IsIrrigated == nil {
		switch p.IrrigationStatus {
		case "irrigated":
			v := true
			p.IsIrrigated = &v
		case "rainfed":
			v := false
			p.IsIrrigated = &v
		}
	}
	if p.WaterBalanceStatus == "" {
		switch p.WaterBalanceRisk {
		case "high", "critical":
			p.WaterBalanceStatus = "under_irrigation_risk"
		default:
			if p.IrrigationStatus == "unknown" {
				p.WaterBalanceStatus = "unknown"
			} else {
				p.WaterBalanceStatus = "balanced"
			}
		}
	}
	var isIrrigated pgtype.Bool
	if p.IsIrrigated != nil {
		isIrrigated = pgtype.Bool{Bool: *p.IsIrrigated, Valid: true}
	}
	var conf, under, over, uniform pgtype.Numeric
	if p.Confidence != nil {
		conf = floatToNumeric(*p.Confidence)
	}
	if p.UnderIrrigationRiskScore != nil {
		under = intToNumeric(*p.UnderIrrigationRiskScore)
	}
	if p.OverIrrigationRiskScore != nil {
		over = intToNumeric(*p.OverIrrigationRiskScore)
	}
	if p.UniformityScore != nil {
		uniform = intToNumeric(*p.UniformityScore)
	}
	return q.InsertTilePredictionIrrigation(ctx, sqlc.InsertTilePredictionIrrigationParams{
		ID:                       id,
		IsIrrigated:              isIrrigated,
		Confidence:               conf,
		WaterBalanceStatus:       mapWaterBalanceStatus(p.WaterBalanceStatus),
		UnderIrrigationRiskScore: under,
		OverIrrigationRiskScore:  over,
		UniformityScore:          uniform,
		Explanations:             jsonStringsToExplanations(p.Explanations),
	})
}

func mapDegradationLevel(s string) sqlc.NullDegradationLevel {
	switch s {
	case "moderate", "medium":
		return sqlc.NullDegradationLevel{DegradationLevel: sqlc.DegradationLevelMedium, Valid: true}
	case "low":
		return sqlc.NullDegradationLevel{DegradationLevel: sqlc.DegradationLevelLow, Valid: true}
	case "high":
		return sqlc.NullDegradationLevel{DegradationLevel: sqlc.DegradationLevelHigh, Valid: true}
	case "severe":
		return sqlc.NullDegradationLevel{DegradationLevel: sqlc.DegradationLevelSevere, Valid: true}
	default:
		return sqlc.NullDegradationLevel{}
	}
}

func mapTrend(s string) sqlc.NullTrendDirection {
	switch s {
	case "degrading", "worsening":
		return sqlc.NullTrendDirection{TrendDirection: sqlc.TrendDirectionWorsening, Valid: true}
	case "improving":
		return sqlc.NullTrendDirection{TrendDirection: sqlc.TrendDirectionImproving, Valid: true}
	case "stable":
		return sqlc.NullTrendDirection{TrendDirection: sqlc.TrendDirectionStable, Valid: true}
	default:
		return sqlc.NullTrendDirection{}
	}
}

func mapAlertLevel(s string) sqlc.NullAlertLevel {
	switch s {
	case "warning", "medium":
		return sqlc.NullAlertLevel{AlertLevel: sqlc.AlertLevelMedium, Valid: true}
	case "low":
		return sqlc.NullAlertLevel{AlertLevel: sqlc.AlertLevelLow, Valid: true}
	case "high":
		return sqlc.NullAlertLevel{AlertLevel: sqlc.AlertLevelHigh, Valid: true}
	case "critical":
		return sqlc.NullAlertLevel{AlertLevel: sqlc.AlertLevelCritical, Valid: true}
	default:
		return sqlc.NullAlertLevel{}
	}
}

func mapWaterBalanceStatus(s string) sqlc.NullWaterBalanceStatus {
	switch s {
	case "under_irrigation_risk", "under_irrigated":
		return sqlc.NullWaterBalanceStatus{WaterBalanceStatus: sqlc.WaterBalanceStatusUnderIrrigated, Valid: true}
	case "over_irrigated":
		return sqlc.NullWaterBalanceStatus{WaterBalanceStatus: sqlc.WaterBalanceStatusOverIrrigated, Valid: true}
	case "balanced":
		return sqlc.NullWaterBalanceStatus{WaterBalanceStatus: sqlc.WaterBalanceStatusBalanced, Valid: true}
	default:
		return sqlc.NullWaterBalanceStatus{}
	}
}

// Ensure TilePredictionPostgres implements ports.MLResultStore.
var _ ports.MLResultStore = (*TilePredictionPostgres)(nil)

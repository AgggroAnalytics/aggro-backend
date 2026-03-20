package workflowfinalize

import (
	"encoding/json"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
)

type meanAgg struct {
	n   int
	sum float64
}

func (a *meanAgg) addInt(v *int) {
	if v == nil {
		return
	}
	a.n++
	a.sum += float64(*v)
}

func (a *meanAgg) addFloat(v *float64) {
	if v == nil {
		return
	}
	a.n++
	a.sum += *v
}

func (a *meanAgg) avg() *float64 {
	if a.n == 0 {
		return nil
	}
	x := a.sum / float64(a.n)
	return &x
}

// AggregateMLResults averages ML JSON payloads (typically one tile × module per item).
// Count returned is the number of non-error payloads unmarshalled successfully.
func AggregateMLResults(raw []json.RawMessage) (ports.PredictedFieldAnalyticsMeans, int) {
	var het, deg, veg, bare, health, stress, water, conf, under, over, uniform meanAgg
	usable := 0

	for _, body := range raw {
		var probe struct {
			Error *string `json:"error"`
		}
		if err := json.Unmarshal(body, &probe); err != nil {
			continue
		}
		if probe.Error != nil && *probe.Error != "" {
			continue
		}

		var p struct {
			HeterogeneityScore       *int `json:"heterogeneity_score"`
			DegradationScore         *int `json:"degradation_score"`
			VegetationCoverLossScore *int `json:"vegetation_cover_loss_score"`
			BareSoilExpansionScore   *int `json:"bare_soil_expansion_score"`
			HealthScore              *int `json:"health_score"`
			StressScoreTotal         *int `json:"stress_score_total"`
			StressScores             *struct {
				WaterStress *int `json:"water_stress"`
			} `json:"stress_scores"`
			Confidence               *float64 `json:"confidence"`
			UnderIrrigationRiskScore *int     `json:"under_irrigation_risk_score"`
			OverIrrigationRiskScore  *int     `json:"over_irrigation_risk_score"`
			UniformityScore          *int     `json:"uniformity_score"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			continue
		}
		usable++
		het.addInt(p.HeterogeneityScore)
		deg.addInt(p.DegradationScore)
		veg.addInt(p.VegetationCoverLossScore)
		bare.addInt(p.BareSoilExpansionScore)
		health.addInt(p.HealthScore)
		stress.addInt(p.StressScoreTotal)
		if p.StressScores != nil {
			water.addInt(p.StressScores.WaterStress)
		}
		conf.addFloat(p.Confidence)
		under.addInt(p.UnderIrrigationRiskScore)
		over.addInt(p.OverIrrigationRiskScore)
		uniform.addInt(p.UniformityScore)
	}

	out := ports.PredictedFieldAnalyticsMeans{
		HeterogeneityScore:       het.avg(),
		DegradationScore:         deg.avg(),
		VegetationCoverLossScore: veg.avg(),
		BareSoilExpansionScore:   bare.avg(),
		HealthScore:              health.avg(),
		StressScoreTotal:         stress.avg(),
		WaterStress:              water.avg(),
		Confidence:               conf.avg(),
		UnderIrrigationRiskScore: under.avg(),
		OverIrrigationRiskScore:  over.avg(),
		UniformityScore:          uniform.avg(),
	}
	return out, usable
}

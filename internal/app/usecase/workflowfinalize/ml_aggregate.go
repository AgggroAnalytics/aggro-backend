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
	var deg, health, stress, water, vegDrop, hetGrowth, conf, events meanAgg
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
			DegradationScore        *int `json:"degradation_score"`
			HealthScore             *int `json:"health_score"`
			StressScoreTotal        *int `json:"stress_score_total"`
			StressScores            *struct {
				WaterStress            *int `json:"water_stress"`
				VegetationActivityDrop *int `json:"vegetation_activity_drop"`
				HeterogeneityGrowth    *int `json:"heterogeneity_growth"`
			} `json:"stress_scores"`
			Confidence              *float64 `json:"confidence"`
			IrrigationEventsDetected *int    `json:"irrigation_events_detected"`
		}
		if err := json.Unmarshal(body, &p); err != nil {
			continue
		}
		usable++
		deg.addInt(p.DegradationScore)
		health.addInt(p.HealthScore)
		stress.addInt(p.StressScoreTotal)
		if p.StressScores != nil {
			water.addInt(p.StressScores.WaterStress)
			vegDrop.addInt(p.StressScores.VegetationActivityDrop)
			hetGrowth.addInt(p.StressScores.HeterogeneityGrowth)
		}
		conf.addFloat(p.Confidence)
		events.addInt(p.IrrigationEventsDetected)
	}

	out := ports.PredictedFieldAnalyticsMeans{
		DegradationScore:         deg.avg(),
		HealthScore:              health.avg(),
		StressScoreTotal:         stress.avg(),
		WaterStress:              water.avg(),
		VegetationActivityDrop:   vegDrop.avg(),
		HeterogeneityGrowth:      hetGrowth.avg(),
		Confidence:               conf.avg(),
		IrrigationEventsDetected: events.avg(),
	}
	return out, usable
}

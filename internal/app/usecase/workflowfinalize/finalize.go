// Package workflowfinalize applies Temporal field-processing workflow results to the backend DB.
package workflowfinalize

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// FieldProcessingCompleteRequest matches workflow-api FieldProcessingResponse (Python worker → backend).
type FieldProcessingCompleteRequest struct {
	FieldID               string          `json:"field_id"`
	Tiles                 []TileInPayload `json:"tiles"`
	Timeseries            []TsRow         `json:"timeseries"`
	MLResults             []json.RawMessage `json:"ml_results"`
	ObservedPmtilesURLs   []DateURL       `json:"observed_pmtiles_urls,omitempty"`
	PredictedPmtilesURLs  []DateURL       `json:"predicted_pmtiles_urls,omitempty"`
}

type TileInPayload struct {
	TileID         string          `json:"tile_id"`
	Geometry       json.RawMessage `json:"geometry"`
	TileGeom       json.RawMessage `json:"tile_geom"`
	GridX          float64         `json:"grid_x"`
	GridY          float64         `json:"grid_y"`
	CoverageRatio  float64         `json:"coverage_ratio"`
	IsClipped      bool            `json:"is_clipped"`
	TileSizeM      float64         `json:"tile_size_m"`
}

type TsRow struct {
	TileID               string   `json:"tile_id"`
	Date                 string   `json:"date"`
	Ndvi                 *float64 `json:"ndvi"`
	Ndmi                 *float64 `json:"ndmi"`
	Ndre                 *float64 `json:"ndre"`
	Gndvi                *float64 `json:"gndvi"`
	Msavi                *float64 `json:"msavi"`
	Nbr2                 *float64 `json:"nbr2"`
	BareSoilIndex        *float64 `json:"bare_soil_index"`
	ValidPixelRatio      *float64 `json:"valid_pixel_ratio"`
	Vv                   *float64 `json:"vv"`
	Vh                   *float64 `json:"vh"`
	PrecipitationMm7d    *float64 `json:"precipitation_mm_7d"`
	PrecipitationMm3d    *float64 `json:"precipitation_mm_3d"`
	PrecipitationMm30d   *float64 `json:"precipitation_mm_30d"`
	TemperatureCMean     *float64 `json:"temperature_c_mean"`
	DryDays              *int     `json:"dry_days"`
}

type DateURL struct {
	Date string `json:"date"`
	URL  string `json:"url"`
}

// ErrFieldNotFound is returned when field_id does not exist.
var ErrFieldNotFound = errors.New("field not found")

type Result struct {
	TilesWritten                  int `json:"tiles_written"`
	TimeseriesRows                int `json:"timeseries_rows"`
	FieldAnalyticsDates           int `json:"field_analytics_dates_updated"`
	FieldPredictedAnalyticsDates  int `json:"field_predicted_analytics_dates_updated"`
	MLResultsApplied              int `json:"ml_results_applied"`
	PmtilesUpserted               int `json:"pmtiles_artifacts_upserted"`
}

type Deps struct {
	Transactor           ports.Transactor
	FieldRepo            ports.FieldRepository
	TileRepo             ports.TileRepository
	TileTsRepo           ports.TileTimeseriesRepository
	FieldAnalyticsRepo   ports.FieldAnalyticsRepository
	MLResultStore        ports.MLResultStore
	PmtilesRepo          ports.AnalysisPmtilesRepository
}

func Apply(ctx context.Context, d Deps, req *FieldProcessingCompleteRequest) (*Result, error) {
	if req == nil {
		return nil, fmt.Errorf("empty request")
	}
	fieldID, err := uuid.Parse(req.FieldID)
	if err != nil {
		return nil, fmt.Errorf("field_id: %w", err)
	}
	f, err := d.FieldRepo.GetFieldByID(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	if f == nil {
		return nil, ErrFieldNotFound
	}
	if len(req.Tiles) == 0 {
		return nil, fmt.Errorf("tiles: at least one tile required")
	}

	res := &Result{}
	err = d.Transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := d.TileRepo.DeleteTilesByFieldID(txCtx, fieldID); err != nil {
			return fmt.Errorf("delete tiles: %w", err)
		}
		for _, t := range req.Tiles {
			tileID, err := uuid.Parse(t.TileID)
			if err != nil {
				return fmt.Errorf("tile_id %q: %w", t.TileID, err)
			}
			wkbBytes, err := geometryToPolygonWKB(t.Geometry)
			if err != nil {
				return fmt.Errorf("tile %s geometry: %w", t.TileID, err)
			}
			if err := d.TileRepo.InsertTileWithID(txCtx, tileID, fieldID, wkbBytes); err != nil {
				return fmt.Errorf("insert tile %s: %w", t.TileID, err)
			}
			res.TilesWritten++
		}

		datesSeen := make(map[time.Time]struct{})
		for _, row := range req.Timeseries {
			tileID, err := uuid.Parse(row.TileID)
			if err != nil {
				continue
			}
			obsDate, err := time.Parse("2006-01-02", row.Date)
			if err != nil {
				continue
			}
			datesSeen[obsDate.UTC().Truncate(24*time.Hour)] = struct{}{}
			var dry *int32
			if row.DryDays != nil {
				v := int32(*row.DryDays)
				dry = &v
			}
			r := &ports.TileTimeseriesRow{
				TileID:             tileID,
				ObservationDate:    obsDate,
				Vh:                 row.Vh,
				Vv:                 row.Vv,
				Nbr2:               row.Nbr2,
				Ndmi:               row.Ndmi,
				Ndre:               row.Ndre,
				Ndvi:               row.Ndvi,
				Gndvi:              row.Gndvi,
				Msavi:              row.Msavi,
				DryDays:            dry,
				BareSoilIndex:      row.BareSoilIndex,
				ValidPixelRatio:    row.ValidPixelRatio,
				TemperatureCMean:   row.TemperatureCMean,
				PrecipitationMm3d:  row.PrecipitationMm3d,
				PrecipitationMm7d:  row.PrecipitationMm7d,
				PrecipitationMm30d: row.PrecipitationMm30d,
			}
			if err := d.TileTsRepo.InsertTileTimeseries(txCtx, r); err != nil {
				return fmt.Errorf("timeseries tile=%s date=%s: %w", row.TileID, row.Date, err)
			}
			res.TimeseriesRows++
		}

		for obsDate := range datesSeen {
			if d.FieldAnalyticsRepo != nil {
				if err := d.FieldAnalyticsRepo.UpsertFieldAnalyticsForFieldAndDate(txCtx, fieldID, obsDate); err != nil {
					return fmt.Errorf("field analytics %s: %w", obsDate.Format("2006-01-02"), err)
				}
				res.FieldAnalyticsDates++
			}
		}

		predMeans, mlUsable := AggregateMLResults(req.MLResults)
		if mlUsable > 0 && d.FieldAnalyticsRepo != nil {
			predMeans.TileCount = int32(mlUsable)
			for obsDate := range datesSeen {
				if err := d.FieldAnalyticsRepo.UpsertFieldPredictedAnalyticsForFieldAndDate(txCtx, fieldID, obsDate, predMeans); err != nil {
					return fmt.Errorf("field predicted analytics %s: %w", obsDate.Format("2006-01-02"), err)
				}
				res.FieldPredictedAnalyticsDates++
			}
		}

		for _, raw := range req.MLResults {
			var probe struct {
				Error *string `json:"error"`
			}
			_ = json.Unmarshal(raw, &probe)
			if probe.Error != nil && *probe.Error != "" {
				continue
			}
			if d.MLResultStore == nil {
				break
			}
			if err := d.MLResultStore.SaveMLCompletion(txCtx, raw); err != nil {
				return fmt.Errorf("ml result: %w", err)
			}
			res.MLResultsApplied++
		}

		for _, row := range req.ObservedPmtilesURLs {
			dt, err := time.Parse("2006-01-02", row.Date)
			if err != nil || row.URL == "" {
				continue
			}
			if err := d.PmtilesRepo.UpsertArtifact(txCtx, fieldID, "observed", dt, "", row.URL); err != nil {
				return fmt.Errorf("observed pmtiles %s: %w", row.Date, err)
			}
			res.PmtilesUpserted++
		}
		for _, row := range req.PredictedPmtilesURLs {
			dt, err := time.Parse("2006-01-02", row.Date)
			if err != nil || row.URL == "" {
				continue
			}
			if err := d.PmtilesRepo.UpsertArtifact(txCtx, fieldID, "prediction", dt, "predicted", row.URL); err != nil {
				return fmt.Errorf("predicted pmtiles %s: %w", row.Date, err)
			}
			res.PmtilesUpserted++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func geometryToPolygonWKB(geojsonBytes json.RawMessage) ([]byte, error) {
	var g geom.T
	if err := geojson.Unmarshal(geojsonBytes, &g); err != nil {
		return nil, err
	}
	switch t := g.(type) {
	case *geom.Polygon:
		return wkb.Marshal(t, binary.BigEndian)
	case *geom.MultiPolygon:
		if t.NumPolygons() == 0 {
			return nil, fmt.Errorf("empty MultiPolygon")
		}
		return wkb.Marshal(t.Polygon(0), binary.BigEndian)
	default:
		return nil, fmt.Errorf("need Polygon or MultiPolygon, got %T", g)
	}
}

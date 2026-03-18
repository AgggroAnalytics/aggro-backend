package consumer

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// TileReplyPayload is the tile-worker reply: field_id (string UUID), tiles array.
type TileReplyPayload struct {
	FieldID string          `json:"field_id"`
	Tiles   []TileReplyItem `json:"tiles"`
}

type TileReplyItem struct {
	TileID   string          `json:"tile_id"`
	Geometry json.RawMessage `json:"geometry"` // GeoJSON
}

// GeoReplyPayload is the geo-worker reply: field_id, timeseries array.
type GeoReplyPayload struct {
	FieldID    string                 `json:"field_id"`
	Timeseries []GeoReplyTimeseriesRow `json:"timeseries"`
}

type GeoReplyTimeseriesRow struct {
	TileID               string   `json:"tile_id"`
	Date                 string   `json:"date"`
	Ndvi                 *float64 `json:"ndvi"`
	Ndmi                 *float64 `json:"ndmi"`
	Ndre                 *float64 `json:"ndre"`
	Gndvi                *float64 `json:"gndvi"`
	Msavi                *float64 `json:"msavi"`
	Vv                   *float64 `json:"vv"`
	Vh                   *float64 `json:"vh"`
	Nbr2                 *float64 `json:"nbr2"`
	BareSoilIndex        *float64 `json:"bare_soil_index"`
	ValidPixelRatio      *float64 `json:"valid_pixel_ratio"`
	DryDays              *int32   `json:"dry_days"`
	TemperatureCMean     *float64 `json:"temperature_c_mean"`
	PrecipitationMm3d    *float64 `json:"precipitation_mm_3d"`
	PrecipitationMm7d    *float64 `json:"precipitation_mm_7d"`
	PrecipitationMm30d   *float64 `json:"precipitation_mm_30d"`
}

// GeoPublisher publishes a payload to the geo-worker exchange.
type GeoPublisher interface {
	PublishGeo(ctx context.Context, body []byte, replyTo, correlationID string) error
}

// ReplyHandlers holds repos and publisher for handling tile/geo/ML replies.
type ReplyHandlers struct {
	TileRepo             ports.TileRepository
	TileTsRepo           ports.TileTimeseriesRepository
	SeasonRepo           ports.SeasonRepository
	FieldAnalyticsRepo   ports.FieldAnalyticsRepository
	GeoPub               GeoPublisher
	MLResultStore        ports.MLResultStore
	PmtilesBuildPublisher ports.PmtilesBuildPublisher
	BackendReplyQueue    string
}

// HandleReply dispatches to tile, geo, or ML handler based on payload.
func (h *ReplyHandlers) HandleReply(ctx context.Context, body []byte) error {
	preview := string(body)
	if len(preview) > 300 {
		preview = preview[:300]
	}
	if IsMLReply(body) {
		slog.Info("[reply] ML reply received", "preview", preview)
		return h.HandleMLReply(ctx, body)
	}
	if IsTileReply(body) {
		slog.Info("[reply] tile reply received", "preview", preview)
		return h.HandleTileReply(ctx, body)
	}
	if IsGeoReply(body) {
		slog.Info("[reply] geo reply received", "preview", preview)
		return h.HandleGeoReply(ctx, body)
	}
	slog.Warn("[reply] unknown reply type", "preview", preview)
	return fmt.Errorf("unknown reply type (no tiles, timeseries, or ML module key)")
}

// IsMLReply returns true if the JSON body looks like an ML module completion (module + result fields).
func IsMLReply(body []byte) bool {
	var m map[string]json.RawMessage
	if json.Unmarshal(body, &m) != nil {
		return false
	}
	if m["module"] == nil {
		return false
	}
	return m["degradation_score"] != nil || m["health_score"] != nil || m["is_irrigated"] != nil
}

// HandleMLReply persists ML completion into tile_predictions tables.
// PMTiles build for predictions is triggered explicitly via POST /fields/{id}/build-pmtiles
// (not here, because N ML replies per field would spam N identical build jobs).
func (h *ReplyHandlers) HandleMLReply(ctx context.Context, body []byte) error {
	if h.MLResultStore == nil {
		return nil
	}
	return h.MLResultStore.SaveMLCompletion(ctx, body)
}

func IsTileReply(body []byte) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(body, &m) == nil && m["tiles"] != nil
}

func IsGeoReply(body []byte) bool {
	var m map[string]json.RawMessage
	return json.Unmarshal(body, &m) == nil && m["timeseries"] != nil
}

// HandleTileReply: save tiles to DB, then optionally send to geo-worker (from_date/to_date from season + max observation).
func (h *ReplyHandlers) HandleTileReply(ctx context.Context, body []byte) error {
	var p TileReplyPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return fmt.Errorf("parse tile reply: %w", err)
	}
	fieldID, err := uuid.Parse(p.FieldID)
	if err != nil {
		return fmt.Errorf("field_id not a UUID: %w", err)
	}

	slog.Info("[reply] HandleTileReply", "field_id", fieldID, "tile_count", len(p.Tiles))

	// Insert each tile (geometry from worker is GeoJSON -> WKB)
	type tileForGeo struct {
		ID     string
		GeoJSON json.RawMessage
	}
	var tilesForGeo []tileForGeo
	for _, t := range p.Tiles {
		wkbBytes, err := geojsonToWKB(t.Geometry)
		if err != nil {
			return fmt.Errorf("tile geometry to WKB: %w", err)
		}
		id, err := h.TileRepo.CreateTile(ctx, fieldID, wkbBytes)
		if err != nil {
			return fmt.Errorf("CreateTile: %w", err)
		}
		tilesForGeo = append(tilesForGeo, tileForGeo{ID: id.String(), GeoJSON: t.Geometry})
	}
	slog.Info("[reply] tiles saved to DB", "field_id", fieldID, "count", len(tilesForGeo))

	// Decide date range from season + max observation date
	seasons, err := h.SeasonRepo.ListSeasonsByFieldID(ctx, fieldID)
	if err != nil {
		return fmt.Errorf("ListSeasonsByFieldID: %w", err)
	}
	if len(seasons) == 0 {
		return nil // no season, skip geo request
	}
	season := seasons[0]
	maxObs, err := h.TileTsRepo.GetMaxObservationDateForField(ctx, fieldID)
	if err != nil {
		return fmt.Errorf("GetMaxObservationDateForField: %w", err)
	}
	today := time.Now().Truncate(24 * time.Hour)
	fromDate := season.StartDate
	if maxObs != nil {
		next := maxObs.Add(24 * time.Hour)
		if next.After(fromDate) {
			fromDate = next
		}
	}
	toDate := season.EndDate
	if toDate.After(today) {
		toDate = today
	}
	if fromDate.After(toDate) {
		return nil // nothing to fetch
	}

	// Build geo-worker payload: tiles with our tile_id (UUID) and geometry
	tilesPayload := make([]map[string]interface{}, 0, len(tilesForGeo))
	for _, t := range tilesForGeo {
		var geom interface{}
		_ = json.Unmarshal(t.GeoJSON, &geom)
		tilesPayload = append(tilesPayload, map[string]interface{}{
			"tile_id":   t.ID,
			"geometry":  geom,
		})
	}
	payload := map[string]interface{}{
		"field_id":   p.FieldID,
		"tiles":     tilesPayload,
		"from_date": fromDate.Format("2006-01-02"),
		"to_date":   toDate.Format("2006-01-02"),
	}
	raw, _ := json.Marshal(payload)
	slog.Info("[reply] publishing geo request", "field_id", fieldID, "tiles", len(tilesForGeo), "from", fromDate.Format("2006-01-02"), "to", toDate.Format("2006-01-02"))
	if err := h.GeoPub.PublishGeo(ctx, raw, h.BackendReplyQueue, ""); err != nil {
		slog.Error("[reply] publish geo FAILED", "field_id", fieldID, "err", err)
		return fmt.Errorf("PublishGeo: %w", err)
	}
	slog.Info("[reply] geo request published OK", "field_id", fieldID)
	return nil
}

// HandleGeoReply: insert each timeseries row, then upsert aggregated field_analytics for each (field_id, date).
func (h *ReplyHandlers) HandleGeoReply(ctx context.Context, body []byte) error {
	var p GeoReplyPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return fmt.Errorf("parse geo reply: %w", err)
	}
	fieldID, _ := uuid.Parse(p.FieldID)
	datesSeen := make(map[time.Time]struct{})
	for _, row := range p.Timeseries {
		tileID, err := uuid.Parse(row.TileID)
		if err != nil {
			continue
		}
		obsDate, err := time.Parse("2006-01-02", row.Date)
		if err != nil {
			continue
		}
		datesSeen[obsDate] = struct{}{}
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
			DryDays:            row.DryDays,
			BareSoilIndex:      row.BareSoilIndex,
			ValidPixelRatio:    row.ValidPixelRatio,
			TemperatureCMean:   row.TemperatureCMean,
			PrecipitationMm3d:  row.PrecipitationMm3d,
			PrecipitationMm7d:  row.PrecipitationMm7d,
			PrecipitationMm30d: row.PrecipitationMm30d,
		}
		if err := h.TileTsRepo.InsertTileTimeseries(ctx, r); err != nil {
			return fmt.Errorf("InsertTileTimeseries: %w", err)
		}
	}
	// Upsert field-level aggregates for each observation date
	if h.FieldAnalyticsRepo != nil && fieldID != uuid.Nil {
		for obsDate := range datesSeen {
			if err := h.FieldAnalyticsRepo.UpsertFieldAnalyticsForFieldAndDate(ctx, fieldID, obsDate); err != nil {
				return fmt.Errorf("UpsertFieldAnalyticsForFieldAndDate: %w", err)
			}
		}
	}
	// Trigger PMTiles build for each date (1 bundle per observed date)
	if h.PmtilesBuildPublisher != nil && fieldID != uuid.Nil {
		for obsDate := range datesSeen {
			_ = h.PmtilesBuildPublisher.PublishBuildJob(ctx, fieldID, "observed", obsDate, "")
		}
	}
	return nil
}

func geojsonToWKB(geojsonBytes json.RawMessage) ([]byte, error) {
	var g geom.T
	if err := geojson.Unmarshal(geojsonBytes, &g); err != nil {
		return nil, err
	}
	return wkb.Marshal(g, binary.BigEndian)
}

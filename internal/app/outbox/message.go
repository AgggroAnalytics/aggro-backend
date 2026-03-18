package outbox

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type OutboxEvent struct {
	ID             uuid.UUID `json:"id"`
	AggregateType  string    `json:"aggregate_type"`
	AggregateID    uuid.UUID `json:"aggregate_id"`
	MessageType    string    `json:"message_type"`
	MessagePayload any       `json:"message_payload"`
	CreatedAt      time.Time `json:"created_at"`
}
type TileGenerationRequest struct {
	FieldID          uuid.UUID       `json:"field_id"`
	FieldGeom        json.RawMessage `json:"field_geom"`
	TileSizeM        int             `json:"tile_size_m,omitempty"`
	MinCoverageRatio float64         `json:"min_coverage_ratio,omitempty"`
	IncludeTileGeom  bool            `json:"include_tile_geom,omitempty"`
}

type TileMetricsRequest struct {
	FieldID  uuid.UUID       `json:"field_id"`
	Tiles    json.RawMessage `json:"field_geom"`
	FromDate string          `json:"from_date,omitempty"`
	ToDate   string          `json:"to_date,omitempty"`
}

func NewTileGenerationEvent(
	fieldID uuid.UUID,
	geojsonGeom []byte,
) (OutboxEvent, error) {

	payload := TileGenerationRequest{
		FieldID:   fieldID,
		FieldGeom: geojsonGeom,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:             uuid.New(),
		AggregateType:  "field",
		AggregateID:    fieldID,
		MessageType:    "tile_generation_requested",
		MessagePayload: data,
		CreatedAt:      time.Now(),
	}, nil
}

func NewTileMetricsEvent(
	fieldID uuid.UUID,
	tiles []byte,
	fromDate time.Time,
	toDate time.Time,
) (OutboxEvent, error) {

	payload := TileMetricsRequest{
		FieldID:  fieldID,
		Tiles:    tiles,
		FromDate: fromDate.Format("YYYY-MM-DD"),
		ToDate:   toDate.Format("YYYY-MM-DD"),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return OutboxEvent{}, err
	}

	return OutboxEvent{
		ID:             uuid.New(),
		AggregateType:  "field",
		AggregateID:    fieldID,
		MessageType:    "tile_metrics_requested",
		MessagePayload: data,
		CreatedAt:      time.Now(),
	}, nil
}

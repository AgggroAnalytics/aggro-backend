package usecase

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
)

type FieldUsecase struct {
	fieldsRepo      ports.FieldRepository
	seasonRepo      ports.SeasonRepository
	tilesPublisher  ports.TilesPublisher
	backendReplyQ   string
}

func NewFieldsUseCase(fieldsRepo ports.FieldRepository, seasonRepo ports.SeasonRepository, tilesPublisher ports.TilesPublisher, backendReplyQueue string) *FieldUsecase {
	return &FieldUsecase{
		fieldsRepo:     fieldsRepo,
		seasonRepo:     seasonRepo,
		tilesPublisher: tilesPublisher,
		backendReplyQ:  backendReplyQueue,
	}
}

func (uc *FieldUsecase) CreateField(ctx context.Context, organizationID uuid.UUID, name string, description string, coordinates [][][]float64) (FieldDTO, error) {

	field := domain.Field{
		Name:           name,
		Description:    description,
		Coordinates:    domain.PolygonFromRings(coordinates),
		OrganizationID: organizationID,
	}

	if err := uc.fieldsRepo.CreateField(ctx, &field); err != nil {
		return FieldDTO{}, err
	}

	// Default season: current year
	yearStart := time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(time.Now().Year(), 12, 31, 23, 59, 59, 0, time.UTC)
	_, _ = uc.seasonRepo.CreateSeason(ctx, field.ID, "Season "+time.Now().Format("2006"), yearStart, yearEnd, true)

	// Send to tile-worker
	geomBytes, err := domain.PolygonToGeoJSONBytes(field.Coordinates)
	if err != nil {
		return FieldDTO{}, err
	}
	payload := map[string]interface{}{
		"field_id":   field.ID.String(),
		"field_geom": json.RawMessage(geomBytes),
	}
	body, _ := json.Marshal(payload)
	if uc.tilesPublisher != nil && uc.backendReplyQ != "" {
		if pubErr := uc.tilesPublisher.PublishTileRequest(ctx, body, uc.backendReplyQ, field.ID.String()); pubErr != nil {
			slog.Error("publish tile request failed", "field_id", field.ID, "err", pubErr)
		} else {
			slog.Info("published tile request", "field_id", field.ID)
		}
	}

	return FieldDTO{ID: field.ID}, nil
}

package usecase

import (
	"context"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
)

type FieldUsecase struct {
	fieldsRepo ports.FieldRepository
	seasonRepo ports.SeasonRepository
}

func NewFieldsUseCase(fieldsRepo ports.FieldRepository, seasonRepo ports.SeasonRepository) *FieldUsecase {
	return &FieldUsecase{
		fieldsRepo: fieldsRepo,
		seasonRepo: seasonRepo,
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

	yearStart := time.Date(time.Now().Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(time.Now().Year(), 12, 31, 23, 59, 59, 0, time.UTC)
	_, _ = uc.seasonRepo.CreateSeason(ctx, field.ID, "Season "+time.Now().Format("2006"), yearStart, yearEnd, true)

	return FieldDTO{ID: field.ID}, nil
}

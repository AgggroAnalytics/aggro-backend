package postgres

import (
	"errors"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/outbox"
	"github.com/google/uuid"
)

func MapDomainEventToOutboxEvent(e domain.DomainEvent) (*outbox.OutboxEvent, error) {
	switch e := e.(type) {
	case domain.FieldCreatedEvent:
		id, _ := uuid.NewV7()
		return &outbox.OutboxEvent{
			ID:            id,
			AggregateType: "field",
			AggregateID:   e.Field_id,
			MessageType:   e.EventName(),
			MessagePayload: struct {
				FieldGeom any
			}{FieldGeom: e.FieldGeom},
		}, nil
	}
	return nil, errors.New("could not map event")
}

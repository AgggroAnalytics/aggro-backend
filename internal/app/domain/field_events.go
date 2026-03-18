package domain

import (
	"time"

	"github.com/google/uuid"
)

type FieldCreatedEvent struct {
	Field_id  uuid.UUID
	FieldGeom Polygon
	CreatedAt time.Time
}

func (e FieldCreatedEvent) EventName() string {
	return "field.created"
}

func (e FieldCreatedEvent) OccuredAt() time.Time {
	return e.CreatedAt
}

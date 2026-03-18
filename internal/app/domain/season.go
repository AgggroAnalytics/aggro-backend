package domain

import (
	"time"

	"github.com/google/uuid"
)

type Season struct {
	ID        uuid.UUID
	FieldID   uuid.UUID
	Name      string
	StartDate time.Time
	EndDate   time.Time
	IsAuto    bool
}

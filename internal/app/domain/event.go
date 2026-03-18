package domain

import "time"

type DomainEvent interface {
	EventName() string
	OccuredAt() time.Time
}

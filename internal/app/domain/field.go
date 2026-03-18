package domain

import (
	"time"

	"github.com/google/uuid"
)

type Point struct {
	Lon float64
	Lat float64
}

type Polygon struct {
	Rings [][]Point
}

// PolygonFromRings builds a Polygon from rings: each ring is [][]float64 (list of [lon, lat]).
// PostGIS requires closed rings (first point = last point). We close any ring that isn't already closed.
func PolygonFromRings(rings [][][]float64) Polygon {
	out := Polygon{Rings: make([][]Point, len(rings))}
	for i, ring := range rings {
		if len(ring) == 0 {
			continue
		}
		closed := ring
		if len(ring) >= 2 {
			first, last := ring[0], ring[len(ring)-1]
			if len(first) >= 2 && len(last) >= 2 && (first[0] != last[0] || first[1] != last[1]) {
				closed = make([][]float64, len(ring)+1)
				copy(closed, ring)
				closed[len(ring)] = ring[0]
			}
		}
		out.Rings[i] = make([]Point, len(closed))
		for j, pt := range closed {
			if len(pt) >= 2 {
				out.Rings[i][j] = Point{Lon: pt[0], Lat: pt[1]}
			}
		}
	}
	return out
}

type Field struct {
	ID             uuid.UUID
	Name           string
	Description    string
	Coordinates    Polygon
	OrganizationID uuid.UUID
	Events         []DomainEvent
}

func NewField(name string, description string, coordinates Polygon, organizationID uuid.UUID) *Field {
	id, _ := uuid.NewV7()

	field := &Field{
		ID:             id,
		Name:           name,
		Description:    description,
		Coordinates:    coordinates,
		OrganizationID: organizationID,
	}
	field.Events = append(field.Events, FieldCreatedEvent{
		Field_id:  id,
		FieldGeom: coordinates,
		CreatedAt: time.Now(),
	})

	return field
}

func (f *Field) PullEvents() []DomainEvent {
	ev := f.Events
	f.Events = nil
	return ev
}

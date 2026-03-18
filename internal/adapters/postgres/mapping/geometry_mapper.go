package postgres

import (
	"encoding/binary"
	"fmt"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkb"
)

// WKBToGeomPolygon decodes PostGIS WKB (e.g. from ST_AsBinary) into *geom.Polygon.
func WKBToGeomPolygon(wkbBytes []byte) (*geom.Polygon, error) {
	g, err := wkb.Unmarshal(wkbBytes)
	if err != nil {
		return nil, fmt.Errorf("wkb unmarshal: %w", err)
	}
	p, ok := g.(*geom.Polygon)
	if !ok {
		return nil, fmt.Errorf("expected polygon, got %T", g)
	}
	return p, nil
}

// GeomPolygonToDomain converts go-geom Polygon to domain.Polygon (for API/domain layer).
func GeomPolygonToDomain(g *geom.Polygon) domain.Polygon {
	var rings [][]domain.Point
	for r := 0; r < g.NumLinearRings(); r++ {
		ring := g.LinearRing(r)
		var ringPts []domain.Point
		for i := 0; i < ring.NumCoords(); i++ {
			c := ring.Coord(i)
			x, y := 0.0, 0.0
			if len(c) >= 2 {
				x, y = c[0], c[1]
			}
			ringPts = append(ringPts, domain.Point{Lon: x, Lat: y})
		}
		rings = append(rings, ringPts)
	}
	return domain.Polygon{Rings: rings}
}

func PolygonToBytea(poly domain.Polygon) ([]byte, error) {

	rings := poly.Rings

	newPoly := geom.NewPolygon(geom.XY)
	for _, ring := range rings {
		geomRing := geom.NewLinearRing(geom.XY)
		var points = make([]geom.Coord, 0, len(ring))
		for _, point := range ring {
			coord := []float64{point.Lon, point.Lat}
			points = append(points, coord)
		}
		geomRing = geomRing.MustSetCoords(points)

		newPoly.Push(geomRing)
	}

	byte, err := wkb.Marshal(newPoly, binary.BigEndian)
	if err != nil {
		return nil, err
	}
	return byte, nil
}

package domain

import (
	"encoding/json"

	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/geojson"
)

// PolygonToGeoJSONBytes returns GeoJSON geometry bytes for the polygon (e.g. for tile-worker field_geom).
func PolygonToGeoJSONBytes(poly Polygon) ([]byte, error) {
	g := polygonToGeom(poly)
	return geojson.Marshal(g)
}

func polygonToGeom(poly Polygon) *geom.Polygon {
	newPoly := geom.NewPolygon(geom.XY)
	for _, ring := range poly.Rings {
		geomRing := geom.NewLinearRing(geom.XY)
		points := make([]geom.Coord, 0, len(ring))
		for _, pt := range ring {
			points = append(points, []float64{pt.Lon, pt.Lat})
		}
		geomRing = geomRing.MustSetCoords(points)
		newPoly.Push(geomRing)
	}
	return newPoly
}

// PolygonToGeoJSONValue returns a value that marshals to GeoJSON (for embedding in another JSON object).
func PolygonToGeoJSONValue(poly Polygon) (json.RawMessage, error) {
	return PolygonToGeoJSONBytes(poly)
}

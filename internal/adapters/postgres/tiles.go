package postgres

import (
	"context"
	"encoding/binary"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/twpayne/go-geom/encoding/wkb"
)

type TilesPostgres struct {
	pool *pgxpool.Pool
}

func NewTilesPostgres(pool *pgxpool.Pool) *TilesPostgres {
	return &TilesPostgres{pool: pool}
}

func (r *TilesPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *TilesPostgres) CreateTile(ctx context.Context, fieldID uuid.UUID, geometryWkb []byte) (uuid.UUID, error) {
	q := r.queries(ctx)
	return q.CreateTile(ctx, sqlc.CreateTileParams{
		FieldID:     fieldID,
		GeometryWkb: geometryWkb,
	})
}

func (r *TilesPostgres) DeleteTilesByFieldID(ctx context.Context, fieldID uuid.UUID) error {
	return r.queries(ctx).DeleteTilesByFieldID(ctx, fieldID)
}

func (r *TilesPostgres) InsertTileWithID(ctx context.Context, tileID, fieldID uuid.UUID, geometryWkb []byte) error {
	return r.queries(ctx).InsertTileWithID(ctx, sqlc.InsertTileWithIDParams{
		ID:          tileID,
		FieldID:     fieldID,
		GeometryWkb: geometryWkb,
	})
}

func (r *TilesPostgres) ListTilesByFieldID(ctx context.Context, fieldID uuid.UUID) ([]ports.TileInfo, error) {
	q := r.queries(ctx)
	tiles, err := q.ListTilesByFieldID(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.TileInfo, 0, len(tiles))
	for _, t := range tiles {
		wkbBytes, err := wkb.Marshal(&t.Geometry, binary.BigEndian)
		if err != nil {
			return nil, err
		}
		out = append(out, ports.TileInfo{ID: t.ID, GeometryWkb: wkbBytes})
	}
	return out, nil
}

func (r *TilesPostgres) ListTileIDsByFieldID(ctx context.Context, fieldID uuid.UUID) ([]uuid.UUID, error) {
	return r.queries(ctx).ListTileIDsByFieldID(ctx, fieldID)
}

func (r *TilesPostgres) ListTilesGeoJSONByFieldID(ctx context.Context, fieldID uuid.UUID) ([]ports.TileGeoJSONRow, error) {
	rows, err := r.queries(ctx).ListTilesGeoJSONByFieldID(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.TileGeoJSONRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.TileGeoJSONRow{ID: row.ID, GeometryJSON: row.GeometryJson})
	}
	return out, nil
}

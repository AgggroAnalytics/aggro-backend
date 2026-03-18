package postgres

import (
	"context"
	"errors"
	"time"

	geomapping "github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/mapping"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FieldsPostgres struct {
	pool *pgxpool.Pool
}

func NewFieldsPostgres(pool *pgxpool.Pool) *FieldsPostgres {
	return &FieldsPostgres{
		pool: pool,
	}
}
func (r *FieldsPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}
func (r *FieldsPostgres) CreateField(ctx context.Context, field *domain.Field) error {

	q := r.queries(ctx)

	if field == nil {
		return errors.New("field is nil")
	}
	id, err := uuid.NewV7()
	if err != nil {
		return errors.New("could not create uuidv7")
	}
	geometryBytea, err := geomapping.PolygonToBytea(field.Coordinates)
	if err != nil {
		return err
	}
	description := pgtype.Text{}
	if field.Description != "" {
		description = pgtype.Text{
			String: field.Description,
			Valid:  true,
		}
	}
	id, err = q.CreateField(ctx, sqlc.CreateFieldParams{
		Name:           field.Name,
		Description:    description,
		GeometryWkb:    geometryBytea,
		ID:             id,
		OrganizationID: field.OrganizationID,
	})

	if err != nil {
		return err
	}
	field.ID = id

	return nil
}

func (r *FieldsPostgres) GetFieldByID(ctx context.Context, id uuid.UUID) (*domain.Field, error) {
	q := r.queries(ctx)
	f, err := q.GetFieldByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	g, err := geomapping.WKBToGeomPolygon(f.GeometryWkb)
	if err != nil {
		return nil, err
	}
	desc := ""
	if f.Description.Valid {
		desc = f.Description.String
	}
	return &domain.Field{
		ID:             f.ID,
		Name:           f.Name,
		Description:    desc,
		Coordinates:    geomapping.GeomPolygonToDomain(g),
		OrganizationID: f.OrganizationID,
	}, nil
}

func timeFromPg(t pgtype.Timestamptz) time.Time {
	if !t.Valid {
		return time.Time{}
	}
	return t.Time
}

func (r *FieldsPostgres) ListFieldsByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]ports.FieldListItem, error) {
	q := r.queries(ctx)
	list, err := q.ListFieldsByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.FieldListItem, 0, len(list))
	for _, row := range list {
		desc := ""
		if row.Description.Valid {
			desc = row.Description.String
		}
		var area *float64
		if row.AreaHectares.Valid {
			af, _ := row.AreaHectares.Float64Value()
			area = &af.Float64
		}
		var coords domain.Polygon
		if len(row.GeometryWkb) > 0 {
			g, err := geomapping.WKBToGeomPolygon(row.GeometryWkb)
			if err == nil {
				coords = geomapping.GeomPolygonToDomain(g)
			}
		}
		out = append(out, ports.FieldListItem{
			ID:             row.ID,
			Name:           row.Name,
			Description:    desc,
			CreatedAt:      timeFromPg(row.CreatedAt),
			AreaHectares:   area,
			OrganizationID: row.OrganizationID,
			Coordinates:    coords,
		})
	}
	return out, nil
}

func (r *FieldsPostgres) UpdateField(ctx context.Context, id uuid.UUID, name, description string) error {
	q := r.queries(ctx)
	desc := pgtype.Text{}
	if description != "" {
		desc = pgtype.Text{String: description, Valid: true}
	}
	return q.UpdateField(ctx, sqlc.UpdateFieldParams{
		ID:          id,
		Name:        name,
		Description: desc,
	})
}

func (r *FieldsPostgres) DeleteField(ctx context.Context, id uuid.UUID) error {
	return r.queries(ctx).DeleteField(ctx, id)
}

package postgres

import (
	"context"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrgDashboardPostgres struct {
	pool *pgxpool.Pool
}

func NewOrgDashboardPostgres(pool *pgxpool.Pool) *OrgDashboardPostgres {
	return &OrgDashboardPostgres{pool: pool}
}

func (r *OrgDashboardPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *OrgDashboardPostgres) Stats(ctx context.Context, organizationID uuid.UUID) (ports.OrgDashboardStats, error) {
	row, err := r.queries(ctx).OrgDashboardStats(ctx, organizationID)
	if err != nil {
		return ports.OrgDashboardStats{}, err
	}
	return ports.OrgDashboardStats{
		FieldCount:                  row.FieldCount,
		TotalAreaHa:                 row.TotalAreaHa,
		FieldsWithObservedAnalytics: row.FieldsWithObservedAnalytics,
		MemberCount:                 row.MemberCount,
	}, nil
}

func (r *OrgDashboardPostgres) ObservedNdviWeekly(ctx context.Context, organizationID uuid.UUID) ([]ports.OrgNdviWeekPoint, error) {
	rows, err := r.queries(ctx).OrgObservedNdviWeekly(ctx, organizationID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.OrgNdviWeekPoint, 0, len(rows))
	for _, row := range rows {
		if !row.WeekStart.Valid {
			continue
		}
		out = append(out, ports.OrgNdviWeekPoint{
			WeekStart:   row.WeekStart.Time,
			NdviMeanAvg: row.NdviMeanAvg,
		})
	}
	return out, nil
}

func (r *OrgDashboardPostgres) StaleFields(ctx context.Context, organizationID uuid.UUID, limit int32) ([]ports.OrgStaleFieldRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 12
	}
	rows, err := r.queries(ctx).OrgFieldsStaleProcessing(ctx, sqlc.OrgFieldsStaleProcessingParams{
		OrganizationID: organizationID,
		RowLimit:       limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.OrgStaleFieldRow, 0, len(rows))
	for _, row := range rows {
		var last *time.Time
		if row.LastAnalyticsAt.Valid {
			t := row.LastAnalyticsAt.Time
			last = &t
		}
		out = append(out, ports.OrgStaleFieldRow{
			FieldID:         row.ID,
			Name:            row.Name,
			LastAnalyticsAt: last,
			TileCount:       row.TileCount,
		})
	}
	return out, nil
}

const orgCentroidSQL = `
SELECT ST_X(c.cent)::float8, ST_Y(c.cent)::float8
FROM (
  SELECT ST_Centroid(ST_Collect(geometry)) AS cent
  FROM fields
  WHERE organization_id = $1
) c
`

// FieldsCentroidWGS84 returns nil lon/lat when there are no fields or centroid is undefined.
func (r *OrgDashboardPostgres) FieldsCentroidWGS84(ctx context.Context, organizationID uuid.UUID) (lon, lat *float64, err error) {
	var lx, ly pgtype.Float8
	err = r.pool.QueryRow(ctx, orgCentroidSQL, organizationID).Scan(&lx, &ly)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	if !lx.Valid || !ly.Valid {
		return nil, nil, nil
	}
	vx, vy := lx.Float64, ly.Float64
	return &vx, &vy, nil
}

var _ ports.OrgDashboardRepository = (*OrgDashboardPostgres)(nil)

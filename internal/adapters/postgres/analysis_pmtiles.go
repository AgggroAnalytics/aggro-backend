package postgres

import (
	"context"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type AnalysisPmtilesPostgres struct {
	pool *pgxpool.Pool
}

func (r *AnalysisPmtilesPostgres) DeleteArtifactsByDates(ctx context.Context, fieldID uuid.UUID, dates []time.Time) error {
	for _, d := range dates {
		day := d.UTC().Truncate(24 * time.Hour)
		if _, err := r.pool.Exec(
			ctx,
			`DELETE FROM analysis_pmtiles_artifacts WHERE field_id = $1 AND analysis_date = $2::date`,
			fieldID,
			day,
		); err != nil {
			return err
		}
	}
	return nil
}

func NewAnalysisPmtilesPostgres(pool *pgxpool.Pool) *AnalysisPmtilesPostgres {
	return &AnalysisPmtilesPostgres{pool: pool}
}

func (r *AnalysisPmtilesPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *AnalysisPmtilesPostgres) ListByFieldID(ctx context.Context, fieldID uuid.UUID) ([]ports.PmtilesArtifactRow, error) {
	list, err := r.queries(ctx).ListAnalysisPmtilesByFieldID(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.PmtilesArtifactRow, 0, len(list))
	for _, row := range list {
		var analysisDate time.Time
		if row.AnalysisDate.Valid {
			analysisDate = row.AnalysisDate.Time
		}
		var createdAt time.Time
		if row.CreatedAt.Valid {
			createdAt = row.CreatedAt.Time
		}
		out = append(out, ports.PmtilesArtifactRow{
			ID:           row.ID,
			FieldID:      row.FieldID,
			AnalysisKind: row.AnalysisKind,
			AnalysisDate: analysisDate,
			Module:       row.Module,
			PmtilesUrl:   row.PmtilesUrl,
			CreatedAt:    createdAt,
		})
	}
	return out, nil
}

func (r *AnalysisPmtilesPostgres) UpsertArtifact(ctx context.Context, fieldID uuid.UUID, analysisKind string, analysisDate time.Time, module, pmtilesURL string) error {
	d := pgtype.Date{Time: analysisDate.UTC().Truncate(24 * time.Hour), Valid: true}
	return r.queries(ctx).UpsertAnalysisPmtilesArtifact(ctx, sqlc.UpsertAnalysisPmtilesArtifactParams{
		FieldID:      fieldID,
		AnalysisKind: analysisKind,
		AnalysisDate: d,
		Module:       module,
		PmtilesUrl:   pmtilesURL,
	})
}

var _ ports.AnalysisPmtilesRepository = (*AnalysisPmtilesPostgres)(nil)

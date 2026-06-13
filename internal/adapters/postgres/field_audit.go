package postgres

import (
	"context"
	"encoding/json"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FieldAuditPostgres struct {
	pool *pgxpool.Pool
}

func NewFieldAuditPostgres(pool *pgxpool.Pool) *FieldAuditPostgres {
	return &FieldAuditPostgres{pool: pool}
}

func (r *FieldAuditPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *FieldAuditPostgres) Insert(ctx context.Context, fieldID, actorUserID uuid.UUID, action string, payload json.RawMessage) error {
	if payload == nil {
		payload = json.RawMessage(`{}`)
	}
	return r.queries(ctx).InsertFieldAuditLog(ctx, sqlc.InsertFieldAuditLogParams{
		FieldID:     fieldID,
		ActorUserID: actorUserID,
		Action:      action,
		Payload:     payload,
	})
}

func (r *FieldAuditPostgres) ListByFieldID(ctx context.Context, fieldID uuid.UUID, limit int) ([]ports.FieldAuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.queries(ctx).ListFieldAuditLogByFieldID(ctx, sqlc.ListFieldAuditLogByFieldIDParams{
		FieldID:  fieldID,
		RowLimit: int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.FieldAuditEntry, 0, len(rows))
	for _, row := range rows {
		var ts time.Time
		if row.CreatedAt.Valid {
			ts = row.CreatedAt.Time
		}
		out = append(out, ports.FieldAuditEntry{
			ID:          row.ID,
			FieldID:     row.FieldID,
			ActorUserID: row.ActorUserID,
			Action:      row.Action,
			Payload:     json.RawMessage(row.Payload),
			CreatedAt:   ts,
		})
	}
	return out, nil
}

func (r *FieldAuditPostgres) ListByOrganizationID(ctx context.Context, organizationID uuid.UUID, limit int) ([]ports.FieldAuditOrgEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.queries(ctx).ListFieldAuditLogByOrganizationID(ctx, sqlc.ListFieldAuditLogByOrganizationIDParams{
		OrganizationID: organizationID,
		RowLimit:       int32(limit),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ports.FieldAuditOrgEntry, 0, len(rows))
	for _, row := range rows {
		var ts time.Time
		if row.CreatedAt.Valid {
			ts = row.CreatedAt.Time
		}
		out = append(out, ports.FieldAuditOrgEntry{
			FieldAuditEntry: ports.FieldAuditEntry{
				ID:          row.ID,
				FieldID:     row.FieldID,
				ActorUserID: row.ActorUserID,
				Action:      row.Action,
				Payload:     json.RawMessage(row.Payload),
				CreatedAt:   ts,
			},
			FieldName: row.FieldName,
		})
	}
	return out, nil
}

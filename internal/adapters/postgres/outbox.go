package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/outbox"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OutboxPostgres struct {
	pool *pgxpool.Pool
}

func NewOutboxPostgres(pool *pgxpool.Pool) *OutboxPostgres {
	return &OutboxPostgres{
		pool: pool,
	}
}

func (r *OutboxPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *OutboxPostgres) CreateOutboxEvent(
	ctx context.Context,
	event *outbox.OutboxEvent) error {
	q := r.queries(ctx)

	var buf bytes.Buffer

	encoder := json.NewEncoder(&buf)

	err := encoder.Encode(event.MessagePayload)

	if err != nil {
		return errors.New("could not encode message payload")
	}

	createdAtPg := pgtype.Timestamptz{
		Valid: true,
		Time:  event.CreatedAt,
	}

	aggIdPg := pgtype.UUID{
		Valid: true,
		Bytes: event.AggregateID,
	}

	aggTypePg := pgtype.Text{
		Valid:  true,
		String: event.AggregateType,
	}
	messageTypePg := pgtype.Text{
		Valid:  true,
		String: event.MessageType,
	}

	params := sqlc.CreateOutboxEventParams{
		ID:             event.ID,
		CreatedAt:      createdAtPg,
		AggregateType:  aggTypePg,
		AggregateID:    aggIdPg,
		MessageType:    messageTypePg,
		MessagePayload: buf.Bytes(),
	}
	err = q.CreateOutboxEvent(ctx, params)
	return err
}

func (r *OrganizationsPostgres) ClaimOneEvent(ctx context.Context) (*outbox.OutboxEvent, error) {
	q := r.queries(ctx)
	row, err := q.ClaimOneEvent(ctx)

	if err != nil {
		return nil, err
	}

	return &outbox.OutboxEvent{
		ID:             row.ID,
		AggregateType:  row.AggregateType.String,
		AggregateID:    row.AggregateID.Bytes,
		MessageType:    row.MessageType.String,
		MessagePayload: row.MessagePayload,
		CreatedAt:      row.CreatedAt.Time,
	}, nil
}

func (r *OrganizationsPostgres) MarkSent(ctx context.Context, id uuid.UUID) error {
	q := r.queries(ctx)
	err := q.MarkSent(ctx, id)

	if err != nil {
		return err
	}

	return nil
}

func (r *OrganizationsPostgres) MarkFailed(ctx context.Context, id uuid.UUID) error {
	q := r.queries(ctx)
	err := q.MarkFailed(ctx, id)

	if err != nil {
		return err
	}

	return nil
}

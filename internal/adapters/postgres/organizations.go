package postgres

import (
	"context"
	"errors"
	"time"

	pgmapping 	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/mapping"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type OrganizationsPostgres struct {
	pool *pgxpool.Pool
}

func NewOrganizationsPostgres(pool *pgxpool.Pool) *OrganizationsPostgres {
	return &OrganizationsPostgres{
		pool: pool,
	}
}

func (r *OrganizationsPostgres) queries(ctx context.Context) *sqlc.Queries {
	if tx, ok := txFromContext(ctx); ok {
		return sqlc.New(tx)
	}
	return sqlc.New(r.pool)
}

func (r *OrganizationsPostgres) ListForUser(ctx context.Context, userID uuid.UUID) ([]ports.OrganizationListItem, error) {
	rows, err := r.queries(ctx).ListOrganizationsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ports.OrganizationListItem, 0, len(rows))
	for _, row := range rows {
		out = append(out, ports.OrganizationListItem{ID: row.ID, Name: row.Name})
	}
	return out, nil
}

func (r *OrganizationsPostgres) CreateOrganization(ctx context.Context, organization *domain.Organization) error {
	q := r.queries(ctx)
	id, err := uuid.NewV7()
	if err != nil {
		return errors.New("could not create uuidv7")
	}

	createdAt := pgtype.Timestamptz{
		Time:  organization.CreatedAt,
		Valid: true,
	}
	params := sqlc.CreateOrganizationParams{
		ID:        id,
		Name:      organization.Name,
		CreatedAt: createdAt,
		CreatedBy: organization.CreatedBy,
	}
	_, err = q.CreateOrganization(ctx, params)
	if err != nil {
		return err
	}
	organization.ID = id
	return nil
}

func (r *OrganizationsPostgres) AddMember(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, role domain.UserRole) error {

	q := r.queries(ctx)
	createdAt := pgtype.Timestamptz{
		Time:  time.Now(),
		Valid: true,
	}

	pgRole := pgmapping.MapDomainRoleToPGRole(role)

	params := sqlc.InviteMemberToOrganizationParams{
		CreatedAt:      createdAt,
		OrganizationID: organizationID,
		UserID:         userID,
		Role:           pgRole,
	}
	err := q.InviteMemberToOrganization(ctx, params)

	return err
}

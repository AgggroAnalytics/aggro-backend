package postgres

import (
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres/sqlc"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
)

func MapDomainRoleToPGRole(role domain.UserRole) sqlc.UserRole {
	switch role {
	case domain.UserRoleAdmin:
		return sqlc.UserRoleAdmin
	case domain.UserRoleFarmer:
		return sqlc.UserRoleFarmer
	case domain.UserRoleManager:
		return sqlc.UserRoleManager
	case domain.UserRoleViewer:
		return sqlc.UserRoleViewer
	default:
		return sqlc.UserRoleViewer
	}
}

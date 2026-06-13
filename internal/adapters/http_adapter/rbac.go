package httpadapter

import (
	"net/http"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/google/uuid"
)

type roleRank int

const (
	rankViewer roleRank = 1
	rankFarmer roleRank = 2
	rankManager roleRank = 3
	rankAdmin roleRank = 4
)

func rankOf(r domain.UserRole) roleRank {
	switch r {
	case domain.UserRoleAdmin:
		return rankAdmin
	case domain.UserRoleManager:
		return rankManager
	case domain.UserRoleFarmer:
		return rankFarmer
	default:
		return rankViewer
	}
}

func (h *handlers) subjectUserID(r *http.Request) (uuid.UUID, bool) {
	sub := SubjectFromContext(r.Context())
	if sub == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(sub)
	if err != nil {
		return uuid.Nil, false
	}
	return id, true
}

// authDisabled is true when JWT middleware is off (no subject in context).
func (h *handlers) authDisabled(r *http.Request) bool {
	return SubjectFromContext(r.Context()) == ""
}

func (h *handlers) ensureOrgRole(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, min domain.UserRole) bool {
	if h.authDisabled(r) {
		return true
	}
	uid, ok := h.subjectUserID(r)
	if !ok {
		h.writeErr(w, http.StatusUnauthorized, "not authenticated")
		return false
	}
	role, member, err := h.d.OrganizationRepo.GetUserRoleInOrganization(r.Context(), uid, orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return false
	}
	if !member {
		h.writeErr(w, http.StatusForbidden, "not a member of this organization")
		return false
	}
	if rankOf(role) < rankOf(min) {
		h.writeErr(w, http.StatusForbidden, "insufficient permissions for this action")
		return false
	}
	return true
}

// ensureFieldRole loads the field and checks org membership; returns the field when allowed.
func (h *handlers) ensureFieldRole(w http.ResponseWriter, r *http.Request, fieldID uuid.UUID, min domain.UserRole) (*domain.Field, bool) {
	f, err := h.d.FieldRepo.GetFieldByID(r.Context(), fieldID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if f == nil {
		h.writeErr(w, http.StatusNotFound, "field not found")
		return nil, false
	}
	if !h.ensureOrgRole(w, r, f.OrganizationID, min) {
		return nil, false
	}
	return f, true
}

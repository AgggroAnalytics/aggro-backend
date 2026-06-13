package httpadapter

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/temporalworkflows"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	"github.com/google/uuid"
)

func (h *handlers) auditField(ctx context.Context, r *http.Request, fieldID uuid.UUID, action string, payload any) {
	if h.d.FieldAuditRepo == nil || h.authDisabled(r) {
		return
	}
	uid, ok := h.subjectUserID(r)
	if !ok {
		return
	}
	b, err := json.Marshal(payload)
	if err != nil {
		b = []byte("{}")
	}
	if err := h.d.FieldAuditRepo.Insert(ctx, fieldID, uid, action, json.RawMessage(b)); err != nil {
		slog.Warn("field audit insert failed", "field_id", fieldID, "action", action, "err", err)
	}
}

// GET /users/me
func (h *handlers) getMe(w http.ResponseWriter, r *http.Request) {
	if h.authDisabled(r) {
		h.writeJSON(w, http.StatusOK, map[string]any{"auth": "disabled"})
		return
	}
	claims := AuthClaimsFromContext(r.Context())
	if claims == nil || claims.Sub == "" {
		h.writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	userID, err := uuid.Parse(claims.Sub)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid user id in token")
		return
	}
	var prefs *ports.UserPreferences
	if h.d.UserPrefsRepo != nil {
		prefs, _ = h.d.UserPrefsRepo.Get(r.Context(), userID)
	}
	if prefs == nil {
		prefs = &ports.UserPreferences{Locale: "ru", Timezone: "UTC", UnitsSystem: "metric", DateFormat: "dmy"}
	}
	u, _ := h.d.UserRepo.GetByID(r.Context(), userID)
	out := map[string]any{
		"id":                  claims.Sub,
		"email":               claims.Email,
		"email_read_only":     true,
		"preferred_username":  claims.PreferredUsername,
		"given_name":          claims.GivenName,
		"family_name":         claims.FamilyName,
		"locale":              prefs.Locale,
		"timezone":            prefs.Timezone,
		"avatar_url":          prefs.AvatarURL,
		"units_system":        prefs.UnitsSystem,
		"date_format":         prefs.DateFormat,
		"fields_default_year": prefs.FieldsDefaultYear,
		"realm_roles":         claims.RealmRoles,
	}
	if u != nil {
		out["username"] = u.Username
		out["first_name"] = u.Firstname
		out["last_name"] = u.LastName
	}
	h.writeJSON(w, http.StatusOK, out)
}

// PATCH /users/me/preferences
func (h *handlers) patchMyPreferences(w http.ResponseWriter, r *http.Request) {
	if h.d.UserPrefsRepo == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "preferences not configured")
		return
	}
	uid, ok := h.subjectUserID(r)
	if !ok {
		h.writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	cur, err := h.d.UserPrefsRepo.Get(r.Context(), uid)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	merge := func(key string, dest *string) {
		if raw, ok := req[key]; ok {
			var s string
			if err := json.Unmarshal(raw, &s); err == nil {
				*dest = s
			}
		}
	}
	merge("locale", &cur.Locale)
	merge("timezone", &cur.Timezone)
	merge("avatar_url", &cur.AvatarURL)
	merge("units_system", &cur.UnitsSystem)
	merge("date_format", &cur.DateFormat)
	if raw, ok := req["fields_default_year"]; ok {
		if string(raw) == "null" {
			cur.FieldsDefaultYear = nil
		} else {
			var y int32
			if err := json.Unmarshal(raw, &y); err == nil {
				cur.FieldsDefaultYear = &y
			}
		}
	}
	cur.UserID = uid
	if err := h.d.UserPrefsRepo.Upsert(r.Context(), cur); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /organizations/{id}/members
func (h *handlers) listOrganizationMembers(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleViewer) {
		return
	}
	members, err := h.d.OrganizationRepo.ListMembers(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(members))
	for _, m := range members {
		items = append(items, map[string]any{
			"user_id":      m.UserID.String(),
			"username":     m.Username,
			"email":        m.Email,
			"first_name":   m.FirstName,
			"last_name":    m.LastName,
			"role":         string(m.Role),
			"member_since": m.MemberSince.Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"members": items})
}

// PATCH /organizations/{id}/members/{userId}
func (h *handlers) patchOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	targetID, ok := h.pathUUID(r, "userId")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleManager) {
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var role domain.UserRole
	switch req.Role {
	case "admin":
		role = domain.UserRoleAdmin
	case "manager":
		role = domain.UserRoleManager
	case "farmer":
		role = domain.UserRoleFarmer
	case "viewer":
		role = domain.UserRoleViewer
	default:
		h.writeErr(w, http.StatusBadRequest, "invalid role")
		return
	}
	if err := h.d.OrganizationRepo.UpdateMemberRole(r.Context(), orgID, targetID, role); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DELETE /organizations/{id}/members/{userId}
func (h *handlers) deleteOrganizationMember(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	targetID, ok := h.pathUUID(r, "userId")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid user id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleManager) {
		return
	}
	if uid, ok := h.subjectUserID(r); ok && targetID == uid {
		h.writeErr(w, http.StatusBadRequest, "cannot remove yourself")
		return
	}
	createdBy, err := h.d.OrganizationRepo.OrganizationCreatedBy(r.Context(), orgID)
	if err == nil && targetID == createdBy {
		h.writeErr(w, http.StatusBadRequest, "cannot remove organization owner")
		return
	}
	if err := h.d.OrganizationRepo.RemoveMember(r.Context(), orgID, targetID); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /organizations/{id}/field-workflow-runs
func (h *handlers) listOrganizationFieldWorkflowRuns(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleViewer) {
		return
	}
	if h.d.TemporalFieldWorkflows == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "workflow listing not configured (set TEMPORAL_ADDRESS)")
		return
	}
	fields, err := h.d.FieldRepo.ListFieldsByOrganizationID(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	type runRow struct {
		FieldID   string                      `json:"field_id"`
		FieldName string                      `json:"field_name"`
		Run       temporalworkflows.FieldRunSummary `json:"run"`
	}
	var all []runRow
	for _, f := range fields {
		runs, err := h.d.TemporalFieldWorkflows.ListFieldProcessingRuns(r.Context(), f.ID)
		if err != nil {
			slog.Warn("list field workflows for org summary", "field_id", f.ID, "err", err)
			continue
		}
		for _, run := range runs {
			all = append(all, runRow{FieldID: f.ID.String(), FieldName: f.Name, Run: run})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		ti, tj := "", ""
		if all[i].Run.StartedAt != nil {
			ti = *all[i].Run.StartedAt
		}
		if all[j].Run.StartedAt != nil {
			tj = *all[j].Run.StartedAt
		}
		return ti > tj
	})
	if len(all) > 200 {
		all = all[:200]
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"organization_id": orgID.String(),
		"runs":            all,
	})
}

// GET /fields/{id}/audit
func (h *handlers) getFieldAudit(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if _, ok := h.ensureFieldRole(w, r, fieldID, domain.UserRoleViewer); !ok {
		return
	}
	if h.d.FieldAuditRepo == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "audit not configured")
		return
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}
	entries, err := h.d.FieldAuditRepo.ListByFieldID(r.Context(), fieldID, limit)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]any, 0, len(entries))
	for _, e := range entries {
		var payload any
		_ = json.Unmarshal(e.Payload, &payload)
		items = append(items, map[string]any{
			"id":            e.ID.String(),
			"field_id":      e.FieldID.String(),
			"actor_user_id": e.ActorUserID.String(),
			"action":        e.Action,
			"payload":       payload,
			"created_at":    e.CreatedAt.Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"field_id": fieldID.String(), "entries": items})
}

// GET /fields/{id}/export?format=csv|geojson&kind=analytics|tiles
func (h *handlers) exportField(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if _, ok := h.ensureFieldRole(w, r, fieldID, domain.UserRoleViewer); !ok {
		return
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	kind := strings.ToLower(r.URL.Query().Get("kind"))
	if format == "" {
		format = "csv"
	}
	if kind == "" {
		kind = "analytics"
	}
	switch {
	case format == "csv" && kind == "analytics":
		var dateFrom, dateTo *time.Time
		if ds := strings.TrimSpace(r.URL.Query().Get("date_from")); ds != "" {
			if t, err := time.ParseInLocation("2006-01-02", ds, time.UTC); err == nil {
				dateFrom = &t
			}
		}
		if ds := strings.TrimSpace(r.URL.Query().Get("date_to")); ds != "" {
			if t, err := time.ParseInLocation("2006-01-02", ds, time.UTC); err == nil {
				end := t.Add(24*time.Hour - time.Nanosecond)
				dateTo = &end
			}
		}
		rows, err := h.d.FieldAnalyticsRepo.ListFieldAnalyticsByFieldID(r.Context(), fieldID, dateFrom, dateTo)
		if err != nil {
			h.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		var buf bytes.Buffer
		cw := csv.NewWriter(&buf)
		_ = cw.Write([]string{
			"observation_date", "source", "ndvi_mean", "ndmi_mean", "ndre_mean", "tile_count",
			"prediction_degradation_score", "prediction_health_score", "prediction_stress_score_total",
		})
		for _, row := range rows {
			rec := []string{
				row.ObservationDate.Format("2006-01-02"),
				row.Source,
				floatStr(row.NdviMean),
				floatStr(row.NdmiMean),
				floatStr(row.NdreMean),
				int32Str(row.TileCount),
				floatStr(row.PredictionDegradationScore),
				floatStr(row.PredictionHealthScore),
				floatStr(row.PredictionStressScoreTotal),
			}
			_ = cw.Write(rec)
		}
		cw.Flush()
		if err := cw.Error(); err != nil {
			h.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		fn := fmt.Sprintf("field_%s_analytics.csv", fieldID.String())
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fn+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	case format == "geojson" && kind == "tiles":
		rows, err := h.d.TileRepo.ListTilesGeoJSONByFieldID(r.Context(), fieldID)
		if err != nil {
			h.writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		features := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			var geom any
			_ = json.Unmarshal(row.GeometryJSON, &geom)
			features = append(features, map[string]any{
				"type":       "Feature",
				"properties": map[string]string{"tile_id": row.ID.String()},
				"geometry":   geom,
			})
		}
		body, _ := json.MarshalIndent(map[string]any{"type": "FeatureCollection", "features": features}, "", "  ")
		fn := fmt.Sprintf("field_%s_tiles.geojson", fieldID.String())
		w.Header().Set("Content-Type", "application/geo+json")
		w.Header().Set("Content-Disposition", `attachment; filename="`+fn+`"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
	default:
		h.writeErr(w, http.StatusBadRequest, "use format=csv|geojson and kind=analytics|tiles")
	}
}

func floatStr(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'g', -1, 64)
}

func int32Str(p *int32) string {
	if p == nil {
		return ""
	}
	return strconv.FormatInt(int64(*p), 10)
}

// GET /fields/{id}/workflows/{runId}/failure
func (h *handlers) getFieldWorkflowFailure(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	runID := r.PathValue("runId")
	if runID == "" {
		h.writeErr(w, http.StatusBadRequest, "invalid run id")
		return
	}
	if _, ok := h.ensureFieldRole(w, r, fieldID, domain.UserRoleViewer); !ok {
		return
	}
	if h.d.TemporalClient == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "temporal not configured")
		return
	}
	wid := temporalworkflows.FieldWorkflowID(fieldID)
	msg := temporalworkflows.WorkflowFailureMessage(r.Context(), h.d.TemporalClient, wid, runID)
	if msg == "" {
		h.writeJSON(w, http.StatusOK, map[string]any{"message": "", "detail": "no failure message on close history"})
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"message": msg})
}

// POST /fields/{id}/workflows/{runId}/terminate
func (h *handlers) terminateFieldWorkflow(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	runID := r.PathValue("runId")
	if runID == "" {
		h.writeErr(w, http.StatusBadRequest, "invalid run id")
		return
	}
	if _, ok := h.ensureFieldRole(w, r, fieldID, domain.UserRoleManager); !ok {
		return
	}
	if h.d.TemporalClient == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "temporal not configured")
		return
	}
	wid := temporalworkflows.FieldWorkflowID(fieldID)
	err := h.d.TemporalClient.TerminateWorkflow(r.Context(), wid, runID, "terminated via API", nil)
	if err != nil {
		h.writeErr(w, http.StatusBadGateway, "temporal terminate: "+err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "terminated"})
}

// POST /fields
func (h *handlers) createField(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req createFieldRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		h.writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	orgID, err := uuid.Parse(req.Organization)
	if err != nil || req.Organization == "" {
		h.writeErr(w, http.StatusBadRequest, "organization_id is required")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleFarmer) {
		return
	}
	dto, err := h.d.FieldUC.CreateField(r.Context(), orgID, req.Name, req.Description, req.Coordinates)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.auditField(r.Context(), r, dto.ID, "field.created", map[string]any{
		"name":             req.Name,
		"organization_id": orgID.String(),
	})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": dto.ID.String()})
}

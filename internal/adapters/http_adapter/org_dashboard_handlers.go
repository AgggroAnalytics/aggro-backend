package httpadapter

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/temporalworkflows"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/google/uuid"
)

type orgWorkflowRunRow struct {
	FieldID   string                            `json:"field_id"`
	FieldName string                            `json:"field_name"`
	Run       temporalworkflows.FieldRunSummary `json:"run"`
}

func (h *handlers) loadOrgWorkflowRuns(ctx context.Context, orgID uuid.UUID, maxFields int) []orgWorkflowRunRow {
	if h.d.TemporalFieldWorkflows == nil || h.d.FieldRepo == nil || maxFields <= 0 {
		return nil
	}
	fields, err := h.d.FieldRepo.ListFieldsByOrganizationID(ctx, orgID)
	if err != nil {
		slog.Warn("dashboard list fields for workflows", "err", err)
		return nil
	}
	if len(fields) > maxFields {
		fields = fields[:maxFields]
	}
	var all []orgWorkflowRunRow
	for _, f := range fields {
		runs, err := h.d.TemporalFieldWorkflows.ListFieldProcessingRuns(ctx, f.ID)
		if err != nil {
			slog.Warn("dashboard list field workflows", "field_id", f.ID, "err", err)
			continue
		}
		for _, run := range runs {
			all = append(all, orgWorkflowRunRow{
				FieldID:   f.ID.String(),
				FieldName: f.Name,
				Run:       run,
			})
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
	return all
}

// GET /organizations/{id}/dashboard
func (h *handlers) getOrganizationDashboard(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleViewer) {
		return
	}
	if h.d.OrgDashboard == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "dashboard not configured")
		return
	}
	ctx := r.Context()
	stats, err := h.d.OrgDashboard.Stats(ctx, orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	ndvi, err := h.d.OrgDashboard.ObservedNdviWeekly(ctx, orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	stale, err := h.d.OrgDashboard.StaleFields(ctx, orgID, 12)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	var seasonTargets json.RawMessage
	if h.d.OrganizationRepo != nil {
		seasonTargets, _ = h.d.OrganizationRepo.GetSeasonTargets(ctx, orgID)
	}
	if len(seasonTargets) == 0 {
		seasonTargets = json.RawMessage(`{}`)
	}

	ndviJSON := make([]map[string]any, 0, len(ndvi))
	var latestNdvi *float64
	for _, p := range ndvi {
		ndviJSON = append(ndviJSON, map[string]any{
			"week_start":    p.WeekStart.UTC().Format(time.RFC3339),
			"ndvi_mean_avg": p.NdviMeanAvg,
		})
	}
	if len(ndvi) > 0 {
		v := ndvi[len(ndvi)-1].NdviMeanAvg
		latestNdvi = &v
	}

	var targetsProgress map[string]any
	var m struct {
		NdviTarget        *float64 `json:"ndvi_target"`
		HealthScoreTarget *float64 `json:"health_score_target"`
		Notes             string   `json:"notes"`
	}
	_ = json.Unmarshal(seasonTargets, &m)
	if m.NdviTarget != nil || m.HealthScoreTarget != nil || latestNdvi != nil {
		targetsProgress = map[string]any{}
		if m.NdviTarget != nil {
			targetsProgress["ndvi_target"] = *m.NdviTarget
			if latestNdvi != nil {
				targetsProgress["latest_ndvi_avg"] = *latestNdvi
				targetsProgress["meets_ndvi"] = *latestNdvi >= *m.NdviTarget
			}
		}
		if m.HealthScoreTarget != nil {
			targetsProgress["health_score_target"] = *m.HealthScoreTarget
		}
		if m.Notes != "" {
			targetsProgress["notes"] = m.Notes
		}
	}

	staleJSON := make([]map[string]any, 0, len(stale))
	for _, s := range stale {
		row := map[string]any{
			"field_id":   s.FieldID.String(),
			"name":       s.Name,
			"tile_count": s.TileCount,
		}
		if s.LastAnalyticsAt != nil {
			row["last_analytics_at"] = s.LastAnalyticsAt.UTC().Format(time.RFC3339)
		} else {
			row["last_analytics_at"] = nil
		}
		staleJSON = append(staleJSON, row)
	}

	runs := h.loadOrgWorkflowRuns(ctx, orgID, 40)
	running := make([]orgWorkflowRunRow, 0)
	var failed []orgWorkflowRunRow
	for _, row := range runs {
		st := strings.ToUpper(row.Run.Status)
		if st == "RUNNING" {
			running = append(running, row)
		}
		if st == "FAILED" && len(failed) < 8 {
			failed = append(failed, row)
		}
	}

	var auditJSON []map[string]any
	if h.d.FieldAuditRepo != nil {
		entries, err := h.d.FieldAuditRepo.ListByOrganizationID(ctx, orgID, 20)
		if err != nil {
			slog.Warn("dashboard audit feed", "err", err)
		} else {
			auditJSON = make([]map[string]any, 0, len(entries))
			for _, e := range entries {
				var payload any
				_ = json.Unmarshal(e.Payload, &payload)
				auditJSON = append(auditJSON, map[string]any{
					"id":            e.ID.String(),
					"field_id":      e.FieldID.String(),
					"field_name":    e.FieldName,
					"actor_user_id": e.ActorUserID.String(),
					"action":        e.Action,
					"payload":       payload,
					"created_at":    e.CreatedAt.UTC().Format(time.RFC3339),
				})
			}
		}
	}

	var season any
	_ = json.Unmarshal(seasonTargets, &season)

	quickFields := make([]map[string]string, 0, 5)
	if h.d.FieldRepo != nil {
		fl, err := h.d.FieldRepo.ListFieldsByOrganizationID(ctx, orgID)
		if err != nil {
			slog.Warn("dashboard quick fields", "err", err)
		} else {
			for i, f := range fl {
				if i >= 5 {
					break
				}
				quickFields = append(quickFields, map[string]string{
					"id":   f.ID.String(),
					"name": f.Name,
				})
			}
		}
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"organization_id": orgID.String(),
		"stats": map[string]any{
			"field_count":                    stats.FieldCount,
			"total_area_ha":                  stats.TotalAreaHa,
			"fields_with_observed_analytics": stats.FieldsWithObservedAnalytics,
			"member_count":                   stats.MemberCount,
		},
		"ndvi_weekly":      ndviJSON,
		"season_targets":   season,
		"targets_progress": targetsProgress,
		"stale_fields":     staleJSON,
		"workflow_running": running,
		"workflow_failed":  failed,
		"recent_audit":     auditJSON,
		"quick_fields":     quickFields,
	})
}

// GET /organizations/{id}/audit-log
func (h *handlers) getOrganizationAuditLog(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleViewer) {
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
	entries, err := h.d.FieldAuditRepo.ListByOrganizationID(r.Context(), orgID, limit)
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
			"field_name":    e.FieldName,
			"actor_user_id": e.ActorUserID.String(),
			"action":        e.Action,
			"payload":       payload,
			"created_at":    e.CreatedAt.UTC().Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"organization_id": orgID.String(),
		"entries":         items,
	})
}

// PATCH /organizations/{id}/season-targets
func (h *handlers) patchOrganizationSeasonTargets(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleManager) {
		return
	}
	if h.d.OrganizationRepo == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "organizations not configured")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 20000))
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid json object")
		return
	}
	if len(body) > 16384 {
		h.writeErr(w, http.StatusBadRequest, "payload too large")
		return
	}
	if err := h.d.OrganizationRepo.UpdateSeasonTargets(r.Context(), orgID, json.RawMessage(body)); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// GET /organizations/{id}/weather — proxy Open-Meteo from org fields centroid (WGS84).
func (h *handlers) getOrganizationWeather(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	if !h.ensureOrgRole(w, r, orgID, domain.UserRoleViewer) {
		return
	}
	if h.d.OrgDashboard == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "dashboard not configured")
		return
	}
	ctx := r.Context()
	lon, lat, err := h.d.OrgDashboard.FieldsCentroidWGS84(ctx, orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if lon == nil || lat == nil {
		h.writeErr(w, http.StatusNotFound, "no field geometry for weather location")
		return
	}
	q := url.Values{}
	q.Set("latitude", strconv.FormatFloat(*lat, 'f', 5, 64))
	q.Set("longitude", strconv.FormatFloat(*lon, 'f', 5, 64))
	q.Set("current", "temperature_2m,relative_humidity_2m,weather_code,wind_speed_10m")
	q.Set("daily", "weather_code,temperature_2m_max,temperature_2m_min,precipitation_sum")
	q.Set("forecast_days", "7")
	q.Set("timezone", "auto")
	u := "https://api.open-meteo.com/v1/forecast?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	client := &http.Client{Timeout: 8 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		h.writeErr(w, http.StatusBadGateway, "weather upstream: "+err.Error())
		return
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		h.writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	if res.StatusCode != http.StatusOK {
		h.writeErr(w, http.StatusBadGateway, "weather upstream status "+strconv.Itoa(res.StatusCode))
		return
	}
	var payload any
	if err := json.Unmarshal(b, &payload); err != nil {
		h.writeErr(w, http.StatusBadGateway, "invalid weather json")
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"latitude":  *lat,
		"longitude": *lon,
		"forecast":  payload,
	})
}

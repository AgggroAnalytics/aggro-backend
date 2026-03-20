package httpadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/temporalworkflows"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/ports"
	fieldusecase "github.com/AgggroAnalytics/aggro-backend/internal/app/usecase/field"
	apispec "github.com/AgggroAnalytics/aggro-backend/openapi"
	"github.com/google/uuid"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

// RouterDeps holds dependencies for HTTP handlers.
type RouterDeps struct {
	FieldUC            *fieldusecase.FieldUsecase
	FieldRepo          ports.FieldRepository
	SeasonRepo         ports.SeasonRepository
	FieldAnalyticsRepo ports.FieldAnalyticsRepository
	PmtilesRepo        ports.AnalysisPmtilesRepository
	TileRepo           ports.TileRepository
	PmtilesBuild       ports.PmtilesBuildPublisher
	OrganizationRepo   ports.OrganizationRepository
	UserRepo           ports.UserRepository
	TileMetricsReader  ports.TileMetricsReader
	TileTsRepo         ports.TileTimeseriesRepository
	MLPublisher        MLPublisher
	S3Client           *s3.Client
	S3Bucket           string
	// TemporalFieldWorkflows lists FieldProcessingWorkflow runs (optional).
	TemporalFieldWorkflows *temporalworkflows.Lister
	TemporalClient         client.Client
	TemporalNamespace      string
	TemporalTaskQueue      string
	TemporalBackendQueue   string
}

func NewRouter(d *RouterDeps) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{d}

	// Health for k8s liveness/readiness
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// OpenAPI spec for client codegen (also reachable without auth — see AuthMiddleware)
	mux.HandleFunc("GET /openapi.yaml", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(apispec.SpecYAML)
	})

	// Fields
	mux.HandleFunc("GET /fields", h.listFields)
	mux.HandleFunc("GET /fields/{id}", h.getField)
	mux.HandleFunc("POST /fields", handleCreateField(d.FieldUC))
	mux.HandleFunc("PUT /fields/{id}", h.updateField)
	mux.HandleFunc("DELETE /fields/{id}", h.deleteField)
	mux.HandleFunc("GET /fields/{id}/analytics", h.getFieldAnalytics)
	mux.HandleFunc("GET /fields/{id}/workflows", h.listFieldWorkflows)
	mux.HandleFunc("POST /fields/{id}/workflows", h.startFieldWorkflow)
	mux.HandleFunc("GET /fields/{id}/processing-dates", h.getFieldProcessingDates)
	mux.HandleFunc("POST /fields/{id}/results/delete", h.deleteFieldResultsByDates)
	mux.HandleFunc("GET /fields/{id}/tiles", h.getFieldTiles)
	mux.HandleFunc("POST /fields/{id}/analytics-jobs", h.postFieldAnalyticsJobs)
	mux.HandleFunc("POST /fields/{id}/build-pmtiles", h.postBuildPmtiles)

	// Seasons
	mux.HandleFunc("GET /seasons", h.listSeasons)
	mux.HandleFunc("GET /seasons/{id}", h.getSeason)
	mux.HandleFunc("POST /seasons", h.createSeason)
	mux.HandleFunc("PUT /seasons/{id}", h.updateSeason)
	mux.HandleFunc("DELETE /seasons/{id}", h.deleteSeason)

	// Organizations
	mux.HandleFunc("GET /organizations", h.listOrganizations)
	mux.HandleFunc("POST /organizations", h.createOrganization)
	mux.HandleFunc("POST /organizations/{id}/invite", h.inviteToOrganization)

	// Tiles (metrics for tooltip)
	mux.HandleFunc("GET /tiles/{id}/metrics", h.getTileMetrics)

	// S3 proxy (PMTiles files from MinIO, with Range support)
	mux.HandleFunc("GET /s3/", h.proxyS3)

	return mux
}

// proxyS3 serves S3 objects via authenticated GetObject (supports Range for PMTiles).
func (h *handlers) proxyS3(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, "/s3/")
	if key == "" {
		h.writeErr(w, http.StatusBadRequest, "missing key")
		return
	}
	if h.d.S3Client == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "S3 not configured")
		return
	}
	bucket := h.d.S3Bucket
	input := &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}
	if rng := r.Header.Get("Range"); rng != "" {
		input.Range = aws.String(rng)
	}
	out, err := h.d.S3Client.GetObject(r.Context(), input)
	if err != nil && strings.Contains(err.Error(), "NoSuchBucket") {
		parts := strings.SplitN(key, "/", 2)
		if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
			input.Bucket = aws.String(parts[0])
			input.Key = aws.String(parts[1])
			out, err = h.d.S3Client.GetObject(r.Context(), input)
		}
	}
	if err != nil {
		slog.Warn("s3 GetObject failed", "key", key, "err", err)
		h.writeErr(w, http.StatusBadGateway, "s3: "+err.Error())
		return
	}
	defer out.Body.Close()
	if out.ContentType != nil {
		w.Header().Set("Content-Type", *out.ContentType)
	}
	if out.ContentLength != nil {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", *out.ContentLength))
	}
	if out.ContentRange != nil {
		w.Header().Set("Content-Range", *out.ContentRange)
	}
	if out.ETag != nil {
		w.Header().Set("ETag", *out.ETag)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if out.ContentRange != nil {
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	io.Copy(w, out.Body)
}

// NewS3Client creates an S3 client for MinIO.
func NewS3Client(endpoint, accessKey, secretKey, region string) *s3.Client {
	creds := credentials.NewStaticCredentialsProvider(accessKey, secretKey, "")
	return s3.New(s3.Options{
		BaseEndpoint: aws.String(endpoint),
		Credentials:  creds,
		Region:       region,
		UsePathStyle: true,
	})
}

type handlers struct {
	d *RouterDeps
}

func (h *handlers) pathUUID(r *http.Request, name string) (uuid.UUID, bool) {
	s := r.PathValue(name)
	if s == "" {
		return uuid.Nil, false
	}
	id, err := uuid.Parse(s)
	return id, err == nil
}

func (h *handlers) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (h *handlers) writeErr(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

// GET /fields?organization_id=uuid
func (h *handlers) listFields(w http.ResponseWriter, r *http.Request) {
	orgStr := r.URL.Query().Get("organization_id")
	if orgStr == "" {
		h.writeErr(w, http.StatusBadRequest, "organization_id query is required")
		return
	}
	orgID, err := uuid.Parse(orgStr)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid organization_id")
		return
	}
	list, err := h.d.FieldRepo.ListFieldsByOrganizationID(r.Context(), orgID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]fieldListItemJSON, 0, len(list))
	for _, f := range list {
		var area *float64
		if f.AreaHectares != nil {
			a := *f.AreaHectares
			area = &a
		}
		items = append(items, fieldListItemJSON{
			ID:             f.ID.String(),
			Name:           f.Name,
			Description:    f.Description,
			CreatedAt:      f.CreatedAt.Format(time.RFC3339),
			AreaHectares:   area,
			OrganizationID: f.OrganizationID.String(),
			Coordinates:    domainPolygonToRings(f.Coordinates),
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"fields": items})
}

type fieldListItemJSON struct {
	ID             string        `json:"id"`
	Name           string        `json:"name"`
	Description    string        `json:"description"`
	CreatedAt      string        `json:"created_at"`
	AreaHectares   *float64      `json:"area_hectares,omitempty"`
	OrganizationID string        `json:"organization_id"`
	Coordinates    [][][]float64 `json:"coordinates"`
}

// GET /fields/{id}
func (h *handlers) getField(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	field, err := h.d.FieldRepo.GetFieldByID(r.Context(), id)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if field == nil {
		h.writeErr(w, http.StatusNotFound, "field not found")
		return
	}
	// Optional: tile count and season count
	tileCount := 0
	if h.d.TileRepo != nil {
		tiles, _ := h.d.TileRepo.ListTilesByFieldID(r.Context(), id)
		tileCount = len(tiles)
	}
	seasonCount := 0
	if h.d.SeasonRepo != nil {
		seasons, _ := h.d.SeasonRepo.ListSeasonsByFieldID(r.Context(), id)
		seasonCount = len(seasons)
	}
	coords := domainPolygonToRings(field.Coordinates)
	h.writeJSON(w, http.StatusOK, map[string]any{
		"id":              field.ID.String(),
		"name":            field.Name,
		"description":     field.Description,
		"coordinates":     coords,
		"organization_id": field.OrganizationID.String(),
		"tile_count":      tileCount,
		"season_count":    seasonCount,
	})
}

// GET /fields/{id}/workflows — FieldProcessingWorkflow runs: status + current stage (from pending activities).
func (h *handlers) listFieldWorkflows(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if h.d.TemporalFieldWorkflows == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "workflow listing not configured (set TEMPORAL_ADDRESS)")
		return
	}
	field, err := h.d.FieldRepo.GetFieldByID(r.Context(), id)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if field == nil {
		h.writeErr(w, http.StatusNotFound, "field not found")
		return
	}
	if sub := SubjectFromContext(r.Context()); sub != "" {
		userID, perr := uuid.Parse(sub)
		if perr != nil {
			h.writeErr(w, http.StatusBadRequest, "invalid user")
			return
		}
		orgs, oerr := h.d.OrganizationRepo.ListForUser(r.Context(), userID)
		if oerr != nil {
			h.writeErr(w, http.StatusInternalServerError, oerr.Error())
			return
		}
		member := false
		for _, o := range orgs {
			if o.ID == field.OrganizationID {
				member = true
				break
			}
		}
		if !member {
			h.writeErr(w, http.StatusForbidden, "access denied")
			return
		}
	}

	runs, err := h.d.TemporalFieldWorkflows.ListFieldProcessingRuns(r.Context(), id)
	if err != nil {
		slog.Error("list field workflows", "field_id", id, "err", err)
		h.writeErr(w, http.StatusBadGateway, "temporal: "+err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"field_id":      id.String(),
		"workflow_id":   temporalworkflows.FieldWorkflowID(id),
		"workflow_type": temporalworkflows.WorkflowTypeFieldProcessing,
		"runs":          runs,
		"listing_note":  "Executions are listed when workflow_id matches field-{field_id} (see run_field_workflow.py).",
	})
}

// POST /fields/{id}/workflows — start FieldProcessingWorkflow in Temporal.
func (h *handlers) startFieldWorkflow(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if h.d.TemporalClient == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "workflow start not configured (set TEMPORAL_ADDRESS)")
		return
	}

	field, err := h.d.FieldRepo.GetFieldByID(r.Context(), id)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if field == nil {
		h.writeErr(w, http.StatusNotFound, "field not found")
		return
	}

	if sub := SubjectFromContext(r.Context()); sub != "" {
		userID, perr := uuid.Parse(sub)
		if perr != nil {
			h.writeErr(w, http.StatusBadRequest, "invalid user")
			return
		}
		orgs, oerr := h.d.OrganizationRepo.ListForUser(r.Context(), userID)
		if oerr != nil {
			h.writeErr(w, http.StatusInternalServerError, oerr.Error())
			return
		}
		member := false
		for _, o := range orgs {
			if o.ID == field.OrganizationID {
				member = true
				break
			}
		}
		if !member {
			h.writeErr(w, http.StatusForbidden, "access denied")
			return
		}
	}

	const dateLayout = "2006-01-02"
	fromDate, toDate, modules, err := h.resolveWorkflowDateRangeAndModules(r.Context(), field.ID, r.Body)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}

	requestPayload := map[string]any{
		"field_id":           id.String(),
		"field_geom":         map[string]any{"type": "Polygon", "coordinates": domainPolygonToRings(field.Coordinates)},
		"tile_size_m":        24,
		"min_coverage_ratio": 0,
		"include_tile_geom":  true,
		"from_date":          fromDate,
		"to_date":            toDate,
		"ml_modules":         modules,
	}
	if h.d.TemporalBackendQueue != "" {
		requestPayload["backend_finalize_task_queue"] = h.d.TemporalBackendQueue
	}

	opts := client.StartWorkflowOptions{
		ID:                    temporalworkflows.FieldWorkflowID(id),
		TaskQueue:             h.d.TemporalTaskQueue,
		WorkflowIDReusePolicy: enumspb.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
	}
	run, err := h.d.TemporalClient.ExecuteWorkflow(r.Context(), opts, temporalworkflows.WorkflowTypeFieldProcessing, requestPayload)
	if err != nil {
		h.writeErr(w, http.StatusBadGateway, "temporal start failed: "+err.Error())
		return
	}

	h.writeJSON(w, http.StatusAccepted, map[string]any{
		"status":        "accepted",
		"field_id":      id.String(),
		"workflow_id":   opts.ID,
		"run_id":        run.GetRunID(),
		"workflow_type": temporalworkflows.WorkflowTypeFieldProcessing,
	})
}

func (h *handlers) getFieldProcessingDates(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}

	now := time.Now().UTC()
	year := now.Year()
	if yearRaw := r.URL.Query().Get("year"); yearRaw != "" {
		parsedYear, perr := strconv.Atoi(yearRaw)
		if perr != nil {
			h.writeErr(w, http.StatusBadRequest, "invalid year")
			return
		}
		year = parsedYear
	}
	yearStart := time.Date(year, time.January, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC)
	if year == now.Year() {
		yearEnd = now
	}
	analytics, err := h.d.FieldAnalyticsRepo.ListFieldAnalyticsByFieldID(r.Context(), fieldID, &yearStart, &yearEnd)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}

	processedSet := make(map[string]struct{}, len(analytics))
	for _, row := range analytics {
		key := row.ObservationDate.UTC().Format("2006-01-02")
		processedSet[key] = struct{}{}
	}

	type processingDateItem struct {
		Date      string `json:"date"`
		Processed bool   `json:"processed"`
	}
	items := make([]processingDateItem, 0)
	for _, d := range buildProcessingDatesForYear(year, yearEnd) {
		key := d
		_, processed := processedSet[key]
		items = append(items, processingDateItem{Date: key, Processed: processed})
	}

	h.writeJSON(w, http.StatusOK, map[string]any{
		"field_id":         fieldID.String(),
		"year":             year,
		"year_start":       yearStart.Format("2006-01-02"),
		"year_end":         yearEnd.Format("2006-01-02"),
		"processing_dates": items,
	})
}

func (h *handlers) pickCurrentSeason(ctx context.Context, fieldID uuid.UUID) (*domain.Season, error) {
	seasons, err := h.d.SeasonRepo.ListSeasonsByFieldID(ctx, fieldID)
	if err != nil {
		return nil, err
	}
	if len(seasons) == 0 {
		return nil, errors.New("no seasons for field")
	}
	sort.Slice(seasons, func(i, j int) bool {
		return seasons[i].StartDate.Before(seasons[j].StartDate)
	})

	now := time.Now().UTC()
	for i := range seasons {
		s := &seasons[i]
		start := s.StartDate.UTC().Truncate(24 * time.Hour)
		end := s.EndDate.UTC().Truncate(24 * time.Hour)
		if !now.Before(start) && !now.After(end) {
			return s, nil
		}
	}
	return &seasons[len(seasons)-1], nil
}

func (h *handlers) resolveWorkflowDateRangeAndModules(ctx context.Context, fieldID uuid.UUID, body io.Reader) (fromDate, toDate string, modules []string, err error) {
	type startWorkflowRequest struct {
		Year             *int     `json:"year"`
		FromDate         string   `json:"from_date"`
		ToDate           string   `json:"to_date"`
		ObservationDates []string `json:"observation_dates"`
		MlModules        []string `json:"ml_modules"`
	}
	req := startWorkflowRequest{}
	if body != nil {
		_ = json.NewDecoder(body).Decode(&req)
	}

	modules = req.MlModules
	if len(modules) == 0 {
		modules = []string{"m0", "m1", "m2"}
	}

	if len(req.ObservationDates) > 0 {
		sorted := append([]string(nil), req.ObservationDates...)
		sort.Strings(sorted)
		fromDate = sorted[0]
		toDate = sorted[len(sorted)-1]
	} else if req.FromDate != "" || req.ToDate != "" {
		fromDate = req.FromDate
		toDate = req.ToDate
	}

	now := time.Now().UTC()
	selectedYear := now.Year()
	if req.Year != nil {
		selectedYear = *req.Year
	}
	yearStart := time.Date(selectedYear, time.January, 1, 0, 0, 0, 0, time.UTC)
	yearEnd := time.Date(selectedYear, time.December, 31, 0, 0, 0, 0, time.UTC)
	if selectedYear == now.Year() {
		yearEnd = now
	}
	analytics, aerr := h.d.FieldAnalyticsRepo.ListFieldAnalyticsByFieldID(ctx, fieldID, &yearStart, &yearEnd)
	if aerr != nil {
		return "", "", nil, aerr
	}
	processedSet := make(map[string]struct{}, len(analytics))
	for _, row := range analytics {
		processedSet[row.ObservationDate.UTC().Format("2006-01-02")] = struct{}{}
	}
	allowedDates := buildProcessingDatesForYear(selectedYear, yearEnd)
	allowedSet := make(map[string]struct{}, len(allowedDates))
	for _, d := range allowedDates {
		allowedSet[d] = struct{}{}
	}

	var selectedDates []string
	if len(req.ObservationDates) > 0 {
		selectedDates = append(selectedDates, req.ObservationDates...)
	} else if req.FromDate != "" || req.ToDate != "" {
		const layout = "2006-01-02"
		start, ferr := time.Parse(layout, req.FromDate)
		if ferr != nil {
			return "", "", nil, errors.New("invalid from_date, expected YYYY-MM-DD")
		}
		end, terr := time.Parse(layout, req.ToDate)
		if terr != nil {
			return "", "", nil, errors.New("invalid to_date, expected YYYY-MM-DD")
		}
		if start.After(end) {
			return "", "", nil, errors.New("from_date must be <= to_date")
		}
		for _, d := range allowedDates {
			dd, _ := time.Parse(layout, d)
			if !dd.Before(start) && !dd.After(end) {
				selectedDates = append(selectedDates, d)
			}
		}
	} else {
		for _, d := range allowedDates {
			if _, processed := processedSet[d]; !processed {
				selectedDates = append(selectedDates, d)
			}
		}
	}

	if len(selectedDates) == 0 {
		return "", "", nil, errors.New("no unprocessed dates to run in current year")
	}
	for _, d := range selectedDates {
		if _, ok := allowedSet[d]; !ok {
			return "", "", nil, errors.New("selected dates are outside allowed schedule")
		}
		if _, processed := processedSet[d]; processed {
			return "", "", nil, errors.New("selected range includes already processed dates")
		}
	}
	sort.Strings(selectedDates)
	fromDate = selectedDates[0]
	toDate = selectedDates[len(selectedDates)-1]

	const layout = "2006-01-02"
	fromParsed, ferr := time.Parse(layout, fromDate)
	if ferr != nil {
		return "", "", nil, errors.New("invalid from_date, expected YYYY-MM-DD")
	}
	toParsed, terr := time.Parse(layout, toDate)
	if terr != nil {
		return "", "", nil, errors.New("invalid to_date, expected YYYY-MM-DD")
	}
	if fromParsed.After(toParsed) {
		return "", "", nil, errors.New("from_date must be <= to_date")
	}
	return fromDate, toDate, modules, nil
}

func buildProcessingDatesForYear(year int, until time.Time) []string {
	anchors := []int{1, 8, 16, 24}
	var out []string
	for month := time.January; month <= time.December; month++ {
		for _, day := range anchors {
			d := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
			if d.Year() != year {
				continue
			}
			if d.After(until) {
				continue
			}
			out = append(out, d.Format("2006-01-02"))
		}
	}
	return out
}

func (h *handlers) deleteFieldResultsByDates(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	var req struct {
		Dates []string `json:"dates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if len(req.Dates) == 0 {
		h.writeErr(w, http.StatusBadRequest, "dates are required")
		return
	}
	const layout = "2006-01-02"
	parsed := make([]time.Time, 0, len(req.Dates))
	for _, dateRaw := range req.Dates {
		d, err := time.Parse(layout, dateRaw)
		if err != nil {
			h.writeErr(w, http.StatusBadRequest, "invalid date format, expected YYYY-MM-DD")
			return
		}
		parsed = append(parsed, d)
	}

	if err := h.d.FieldAnalyticsRepo.DeleteFieldAnalyticsByDates(r.Context(), fieldID, parsed); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.d.PmtilesRepo.DeleteArtifactsByDates(r.Context(), fieldID, parsed); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"status": "deleted",
		"dates":  req.Dates,
	})
}

func domainPolygonToRings(p domain.Polygon) [][][]float64 {
	out := make([][][]float64, 0, len(p.Rings))
	for _, ring := range p.Rings {
		r := make([][]float64, 0, len(ring))
		for _, pt := range ring {
			r = append(r, []float64{pt.Lon, pt.Lat})
		}
		out = append(out, r)
	}
	return out
}

// PUT /fields/{id}
func (h *handlers) updateField(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		h.writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := h.d.FieldRepo.UpdateField(r.Context(), id, req.Name, req.Description); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"id": id.String()})
}

// DELETE /fields/{id}
func (h *handlers) deleteField(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if err := h.d.FieldRepo.DeleteField(r.Context(), id); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /fields/{id}/tiles — GeoJSON FeatureCollection of tiles for field (for map layer + click).
func (h *handlers) getFieldTiles(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	rows, err := h.d.TileRepo.ListTilesGeoJSONByFieldID(r.Context(), id)
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
	h.writeJSON(w, http.StatusOK, map[string]any{"type": "FeatureCollection", "features": features})
}

// GET /fields/{id}/analytics?date_from=...&date_to=...
func (h *handlers) getFieldAnalytics(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	var dateFrom, dateTo *time.Time
	if s := r.URL.Query().Get("date_from"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			h.writeErr(w, http.StatusBadRequest, "invalid date_from")
			return
		}
		dateFrom = &t
	}
	if s := r.URL.Query().Get("date_to"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			h.writeErr(w, http.StatusBadRequest, "invalid date_to")
			return
		}
		dateTo = &t
	}
	rows, err := h.d.FieldAnalyticsRepo.ListFieldAnalyticsByFieldID(r.Context(), id, dateFrom, dateTo)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	pmtiles, _ := h.d.PmtilesRepo.ListByFieldID(r.Context(), id)
	analytics := make([]fieldAnalyticsRowJSON, 0, len(rows))
	for _, row := range rows {
		analytics = append(analytics, fieldAnalyticsRowToJSON(row))
	}
	artifacts := make([]pmtilesArtifactJSON, 0, len(pmtiles))
	for _, a := range pmtiles {
		artifacts = append(artifacts, pmtilesArtifactJSON{
			ID:           a.ID.String(),
			FieldID:      a.FieldID.String(),
			AnalysisKind: a.AnalysisKind,
			AnalysisDate: a.AnalysisDate.Format("2006-01-02"),
			Module:       a.Module,
			PmtilesUrl:   a.PmtilesUrl,
			CreatedAt:    a.CreatedAt.Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"field_id":  id.String(),
		"analytics": analytics,
		"pmtiles":   artifacts,
	})
}

func fieldAnalyticsRowToJSON(row ports.FieldAnalyticsRow) fieldAnalyticsRowJSON {
	j := fieldAnalyticsRowJSON{
		ID:                                 row.ID.String(),
		FieldID:                            row.FieldID.String(),
		ObservationDate:                    row.ObservationDate.Format("2006-01-02"),
		Source:                             row.Source,
		TileCount:                          row.TileCount,
		ValidTileCount:                     row.ValidTileCount,
		NdviMean:                           row.NdviMean,
		NdmiMean:                           row.NdmiMean,
		NdreMean:                           row.NdreMean,
		GndviMean:                          row.GndviMean,
		MsaviMean:                          row.MsaviMean,
		Nbr2Mean:                           row.Nbr2Mean,
		BareSoilIndexMean:                  row.BareSoilIndexMean,
		ValidPixelRatioMean:                row.ValidPixelRatioMean,
		StressIndexMean:                    row.StressIndexMean,
		TemperatureCMean:                   row.TemperatureCMean,
		PrecipitationMm3dMean:              row.PrecipitationMm3dMean,
		PrecipitationMm7dMean:              row.PrecipitationMm7dMean,
		PrecipitationMm30dMean:             row.PrecipitationMm30dMean,
		HeterogeneityScore:                 row.HeterogeneityScore,
		PredictionDegradationScore:         row.PredictionDegradationScore,
		PredictionVegetationCoverLossScore: row.PredictionVegetationCoverLossScore,
		PredictionBareSoilExpansionScore:   row.PredictionBareSoilExpansionScore,
		PredictionHealthScore:              row.PredictionHealthScore,
		PredictionStressScoreTotal:         row.PredictionStressScoreTotal,
		PredictionWaterStress:              row.PredictionWaterStress,
		PredictionConfidence:               row.PredictionConfidence,
		PredictionUnderIrrigationRiskScore: row.PredictionUnderIrrigationRiskScore,
		PredictionOverIrrigationRiskScore:  row.PredictionOverIrrigationRiskScore,
		PredictionUniformityScore:          row.PredictionUniformityScore,
		CreatedAt:                          row.CreatedAt.Format(time.RFC3339),
	}
	return j
}

type fieldAnalyticsRowJSON struct {
	ID                                 string   `json:"id"`
	FieldID                            string   `json:"field_id"`
	ObservationDate                    string   `json:"observation_date"`
	Source                             string   `json:"source"`
	TileCount                          *int32   `json:"tile_count,omitempty"`
	ValidTileCount                     *int32   `json:"valid_tile_count,omitempty"`
	NdviMean                           *float64 `json:"ndvi_mean,omitempty"`
	NdmiMean                           *float64 `json:"ndmi_mean,omitempty"`
	NdreMean                           *float64 `json:"ndre_mean,omitempty"`
	GndviMean                          *float64 `json:"gndvi_mean,omitempty"`
	MsaviMean                          *float64 `json:"msavi_mean,omitempty"`
	Nbr2Mean                           *float64 `json:"nbr2_mean,omitempty"`
	BareSoilIndexMean                  *float64 `json:"bare_soil_index_mean,omitempty"`
	ValidPixelRatioMean                *float64 `json:"valid_pixel_ratio_mean,omitempty"`
	StressIndexMean                    *float64 `json:"stress_index_mean,omitempty"`
	TemperatureCMean                   *float64 `json:"temperature_c_mean,omitempty"`
	PrecipitationMm3dMean              *float64 `json:"precipitation_mm_3d_mean,omitempty"`
	PrecipitationMm7dMean              *float64 `json:"precipitation_mm_7d_mean,omitempty"`
	PrecipitationMm30dMean             *float64 `json:"precipitation_mm_30d_mean,omitempty"`
	HeterogeneityScore                 *float64 `json:"heterogeneity_score,omitempty"`
	PredictionDegradationScore         *float64 `json:"prediction_degradation_score,omitempty"`
	PredictionVegetationCoverLossScore *float64 `json:"prediction_vegetation_cover_loss_score,omitempty"`
	PredictionBareSoilExpansionScore   *float64 `json:"prediction_bare_soil_expansion_score,omitempty"`
	PredictionHealthScore              *float64 `json:"prediction_health_score,omitempty"`
	PredictionStressScoreTotal         *float64 `json:"prediction_stress_score_total,omitempty"`
	PredictionWaterStress              *float64 `json:"prediction_water_stress,omitempty"`
	PredictionConfidence               *float64 `json:"prediction_confidence,omitempty"`
	PredictionUnderIrrigationRiskScore *float64 `json:"prediction_under_irrigation_risk_score,omitempty"`
	PredictionOverIrrigationRiskScore  *float64 `json:"prediction_over_irrigation_risk_score,omitempty"`
	PredictionUniformityScore          *float64 `json:"prediction_uniformity_score,omitempty"`
	CreatedAt                          string   `json:"created_at"`
}

type pmtilesArtifactJSON struct {
	ID           string `json:"id"`
	FieldID      string `json:"field_id"`
	AnalysisKind string `json:"analysis_kind"`
	AnalysisDate string `json:"analysis_date"`
	Module       string `json:"module"`
	PmtilesUrl   string `json:"pmtiles_url"`
	CreatedAt    string `json:"created_at"`
}

// MLPublisher publishes analytics jobs to the ML worker exchange.
type MLPublisher interface {
	PublishJSON(ctx context.Context, body []byte, replyTo, correlationID string) error
}

// POST /fields/{id}/analytics-jobs — fetch tiles + timeseries, publish ML jobs for m0/m1/m2
func (h *handlers) postFieldAnalyticsJobs(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if h.d.MLPublisher == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "ML publisher not configured")
		return
	}
	tileIDs, err := h.d.TileRepo.ListTileIDsByFieldID(r.Context(), fieldID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(tileIDs) == 0 {
		h.writeErr(w, http.StatusBadRequest, "no tiles for this field; create field first")
		return
	}

	modules := []string{"m0", "m1", "m2"}
	published := 0
	tilesWithTs := 0
	replyQueue := ""
	if h.d.PmtilesBuild != nil {
		replyQueue = "field.backend.replies"
	}

	slog.Info("[analytics-jobs] starting", "field_id", fieldID, "tile_count", len(tileIDs))

	for _, tileID := range tileIDs {
		ts, err := h.d.TileTsRepo.ListByTileID(r.Context(), tileID)
		if err != nil {
			slog.Error("list timeseries for ML job", "tile_id", tileID, "err", err)
			continue
		}
		if len(ts) == 0 {
			slog.Info("[analytics-jobs] tile has no timeseries", "tile_id", tileID)
			continue
		}
		tilesWithTs++
		tsJSON := make([]map[string]interface{}, 0, len(ts))
		for _, row := range ts {
			entry := map[string]interface{}{
				"date": row.ObservationDate.Format("2006-01-02"),
			}
			if row.Ndvi != nil {
				entry["ndvi"] = *row.Ndvi
			}
			if row.Ndmi != nil {
				entry["ndmi"] = *row.Ndmi
			}
			if row.Ndre != nil {
				entry["ndre"] = *row.Ndre
			}
			if row.Gndvi != nil {
				entry["gndvi"] = *row.Gndvi
			}
			if row.Msavi != nil {
				entry["msavi"] = *row.Msavi
			}
			if row.Vv != nil {
				entry["vv"] = *row.Vv
			}
			if row.Vh != nil {
				entry["vh"] = *row.Vh
			}
			if row.Nbr2 != nil {
				entry["nbr2"] = *row.Nbr2
			}
			if row.BareSoilIndex != nil {
				entry["bare_soil_index"] = *row.BareSoilIndex
			}
			if row.ValidPixelRatio != nil {
				entry["valid_pixel_ratio"] = *row.ValidPixelRatio
			}
			if row.DryDays != nil {
				entry["dry_days"] = *row.DryDays
			}
			if row.TemperatureCMean != nil {
				entry["temperature_c_mean"] = *row.TemperatureCMean
			}
			if row.PrecipitationMm3d != nil {
				entry["precipitation_mm_3d"] = *row.PrecipitationMm3d
			}
			if row.PrecipitationMm7d != nil {
				entry["precipitation_mm_7d"] = *row.PrecipitationMm7d
			}
			if row.PrecipitationMm30d != nil {
				entry["precipitation_mm_30d"] = *row.PrecipitationMm30d
			}
			tsJSON = append(tsJSON, entry)
		}

		for _, module := range modules {
			payload := map[string]interface{}{
				"module":     module,
				"field_id":   fieldID.String(),
				"tile_id":    tileID.String(),
				"timeseries": tsJSON,
			}
			body, _ := json.Marshal(payload)
			if pubErr := h.d.MLPublisher.PublishJSON(r.Context(), body, replyQueue, ""); pubErr != nil {
				slog.Error("publish ML job", "module", module, "tile_id", tileID, "err", pubErr)
			} else {
				published++
			}
		}
	}

	slog.Info("[analytics-jobs] published ML jobs", "field_id", fieldID, "tiles_total", len(tileIDs), "tiles_with_ts", tilesWithTs, "jobs_sent", published)
	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":        "accepted",
		"tiles_total":   len(tileIDs),
		"tiles_with_ts": tilesWithTs,
		"jobs_sent":     published,
	})
}

// POST /fields/{id}/build-pmtiles — trigger PMTiles build for prediction layers
func (h *handlers) postBuildPmtiles(w http.ResponseWriter, r *http.Request) {
	fieldID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid field id")
		return
	}
	if h.d.PmtilesBuild == nil {
		h.writeErr(w, http.StatusServiceUnavailable, "PMTiles publisher not configured")
		return
	}
	today := time.Now().UTC().Truncate(24 * time.Hour)
	modules := []string{"degradation", "health_stress", "irrigation_water_use"}
	published := 0
	for _, mod := range modules {
		if err := h.d.PmtilesBuild.PublishBuildJob(r.Context(), fieldID, "prediction", today, mod); err != nil {
			slog.Error("publish pmtiles build", "field_id", fieldID, "module", mod, "err", err)
		} else {
			published++
		}
	}
	slog.Info("[build-pmtiles] triggered", "field_id", fieldID, "jobs", published)
	h.writeJSON(w, http.StatusAccepted, map[string]interface{}{
		"status":    "accepted",
		"jobs_sent": published,
	})
}

// GET /seasons?field_id=uuid
func (h *handlers) listSeasons(w http.ResponseWriter, r *http.Request) {
	fieldStr := r.URL.Query().Get("field_id")
	if fieldStr == "" {
		h.writeErr(w, http.StatusBadRequest, "field_id query is required")
		return
	}
	fieldID, err := uuid.Parse(fieldStr)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid field_id")
		return
	}
	list, err := h.d.SeasonRepo.ListSeasonsByFieldID(r.Context(), fieldID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]seasonJSON, 0, len(list))
	for _, s := range list {
		items = append(items, seasonJSON{
			ID:        s.ID.String(),
			FieldID:   s.FieldID.String(),
			Name:      s.Name,
			StartDate: s.StartDate.Format("2006-01-02"),
			EndDate:   s.EndDate.Format("2006-01-02"),
			IsAuto:    s.IsAuto,
		})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"seasons": items})
}

// GET /seasons/{id}
func (h *handlers) getSeason(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid season id")
		return
	}
	s, err := h.d.SeasonRepo.GetSeasonByID(r.Context(), id)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		h.writeErr(w, http.StatusNotFound, "season not found")
		return
	}
	h.writeJSON(w, http.StatusOK, seasonJSON{
		ID:        s.ID.String(),
		FieldID:   s.FieldID.String(),
		Name:      s.Name,
		StartDate: s.StartDate.Format("2006-01-02"),
		EndDate:   s.EndDate.Format("2006-01-02"),
		IsAuto:    s.IsAuto,
	})
}

type seasonJSON struct {
	ID        string `json:"id"`
	FieldID   string `json:"field_id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	IsAuto    bool   `json:"is_auto"`
}

// POST /seasons
func (h *handlers) createSeason(w http.ResponseWriter, r *http.Request) {
	var req struct {
		FieldID   string `json:"field_id"`
		Name      string `json:"name"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		IsAuto    bool   `json:"is_auto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.FieldID == "" || req.Name == "" || req.StartDate == "" || req.EndDate == "" {
		h.writeErr(w, http.StatusBadRequest, "field_id, name, start_date, end_date are required")
		return
	}
	fieldID, err := uuid.Parse(req.FieldID)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid field_id")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid start_date")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid end_date")
		return
	}
	id, err := h.d.SeasonRepo.CreateSeason(r.Context(), fieldID, req.Name, startDate, endDate, req.IsAuto)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id.String()})
}

// PUT /seasons/{id}
func (h *handlers) updateSeason(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid season id")
		return
	}
	var req struct {
		Name      string `json:"name"`
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		IsAuto    bool   `json:"is_auto"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.StartDate == "" || req.EndDate == "" {
		h.writeErr(w, http.StatusBadRequest, "name, start_date, end_date are required")
		return
	}
	startDate, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid start_date")
		return
	}
	endDate, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid end_date")
		return
	}
	if err := h.d.SeasonRepo.UpdateSeason(r.Context(), id, req.Name, startDate, endDate, req.IsAuto); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusOK, map[string]string{"id": id.String()})
}

// DELETE /seasons/{id}
func (h *handlers) deleteSeason(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid season id")
		return
	}
	if err := h.d.SeasonRepo.DeleteSeason(r.Context(), id); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /tiles/{id}/metrics — observed timeseries + ML predictions for tile (tooltip).
func (h *handlers) getTileMetrics(w http.ResponseWriter, r *http.Request) {
	id, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid tile id")
		return
	}
	if h.d.TileMetricsReader == nil {
		h.writeErr(w, http.StatusNotImplemented, "tile metrics not configured")
		return
	}
	metrics, err := h.d.TileMetricsReader.GetTileMetrics(r.Context(), id)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if metrics == nil {
		h.writeErr(w, http.StatusNotFound, "tile not found")
		return
	}
	observed := make([]map[string]any, 0, len(metrics.Observed))
	for _, o := range metrics.Observed {
		observed = append(observed, map[string]any{
			"observation_date":     o.ObservationDate.Format(time.RFC3339),
			"ndvi":                 o.Ndvi,
			"ndmi":                 o.Ndmi,
			"ndre":                 o.Ndre,
			"valid_pixel_ratio":    o.ValidPixelRatio,
			"stress_index":         o.StressIndex,
			"temperature_c_mean":   o.TemperatureCMean,
			"precipitation_mm_3d":  o.PrecipitationMm3d,
			"precipitation_mm_7d":  o.Mm7d,
			"precipitation_mm_30d": o.Mm30d,
		})
	}
	predictions := make([]map[string]any, 0, len(metrics.Predictions))
	for _, p := range metrics.Predictions {
		m := map[string]any{
			"id":              p.ID.String(),
			"module":          p.Module,
			"prediction_date": p.PredictionDate.Format(time.RFC3339),
			"status":          p.Status,
		}
		if p.Degradation != nil {
			m["degradation"] = p.Degradation
		}
		if p.HealthStress != nil {
			m["health_stress"] = p.HealthStress
		}
		if p.Irrigation != nil {
			m["irrigation"] = p.Irrigation
		}
		predictions = append(predictions, m)
	}
	h.writeJSON(w, http.StatusOK, map[string]any{
		"tile_id":     metrics.TileID.String(),
		"observed":    observed,
		"predictions": predictions,
	})
}

// GET /organizations — list organizations for the current user (created by or member).
func (h *handlers) listOrganizations(w http.ResponseWriter, r *http.Request) {
	sub := SubjectFromContext(r.Context())
	if sub == "" {
		h.writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	userID, err := uuid.Parse(sub)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid user")
		return
	}
	list, err := h.d.OrganizationRepo.ListForUser(r.Context(), userID)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := make([]map[string]string, 0, len(list))
	for _, o := range list {
		items = append(items, map[string]string{"id": o.ID.String(), "name": o.Name})
	}
	h.writeJSON(w, http.StatusOK, map[string]any{"organizations": items})
}

// POST /organizations — create organization (created_by = authenticated user from JWT).
func (h *handlers) createOrganization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.writeErr(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		h.writeErr(w, http.StatusBadRequest, "name is required")
		return
	}
	sub := SubjectFromContext(r.Context())
	if sub == "" {
		h.writeErr(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	createdBy, err := uuid.Parse(sub)
	if err != nil {
		h.writeErr(w, http.StatusBadRequest, "invalid created_by_user_id")
		return
	}
	org := &domain.Organization{
		Name:      req.Name,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}
	if err := h.d.OrganizationRepo.CreateOrganization(r.Context(), org); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"id": org.ID.String()})
}

// POST /organizations/{id}/invite — invite a registered user by email.
func (h *handlers) inviteToOrganization(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.pathUUID(r, "id")
	if !ok {
		h.writeErr(w, http.StatusBadRequest, "invalid organization id")
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Email == "" {
		h.writeErr(w, http.StatusBadRequest, "email is required")
		return
	}
	role := domain.UserRoleViewer
	if req.Role != "" {
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
			h.writeErr(w, http.StatusBadRequest, "invalid role: use admin, manager, farmer, or viewer")
			return
		}
	}
	user, err := h.d.UserRepo.GetByEmail(r.Context(), req.Email)
	if err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if user == nil {
		h.writeErr(w, http.StatusNotFound, "user not found with this email")
		return
	}
	if err := h.d.OrganizationRepo.AddMember(r.Context(), orgID, user.ID, role); err != nil {
		h.writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.writeJSON(w, http.StatusCreated, map[string]string{
		"organization_id": orgID.String(),
		"user_id":         user.ID.String(),
		"role":            string(role),
	})
}

// --- Create field (unchanged) ---

type createFieldRequest struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	Coordinates  [][][]float64 `json:"coordinates"`
	Organization string        `json:"organization_id"`
}

func handleCreateField(fieldUC *fieldusecase.FieldUsecase) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req createFieldRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var orgID uuid.UUID
		if req.Organization != "" {
			orgID, _ = uuid.Parse(req.Organization)
		}
		dto, err := fieldUC.CreateField(r.Context(), orgID, req.Name, req.Description, req.Coordinates)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": dto.ID.String()})
	}
}

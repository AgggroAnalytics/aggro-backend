package ports

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/google/uuid"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	PrepareContext(ctx context.Context, query string) (*sql.Stmt, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type FieldRepository interface {
	CreateField(ctx context.Context, field *domain.Field) error
	GetFieldByID(ctx context.Context, id uuid.UUID) (*domain.Field, error)
	ListFieldsByOrganizationID(ctx context.Context, organizationID uuid.UUID) ([]FieldListItem, error)
	UpdateField(ctx context.Context, id uuid.UUID, name, description string) error
	DeleteField(ctx context.Context, id uuid.UUID) error
}

// FieldListItem is a field row for list (includes coordinates as number[][][]).
type FieldListItem struct {
	ID                     uuid.UUID
	Name                   string
	Description            string
	CreatedAt              time.Time
	AreaHectares           *float64
	OrganizationID         uuid.UUID
	Coordinates            domain.Polygon
	TileCount              int32
	SeasonCount            int32
	LatestObservationAt    *time.Time
	ObservedAnalyticsDates int32
	PmtilesLayerCount      int32
}

// OrganizationListItem is a row for list APIs.
type OrganizationListItem struct {
	ID   uuid.UUID
	Name string
}

// OrganizationMember is a user row with role in an organization.
type OrganizationMember struct {
	UserID      uuid.UUID
	Username    string
	Email       string
	FirstName   string
	LastName    string
	Role        domain.UserRole
	MemberSince time.Time
}

type OrganizationRepository interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]OrganizationListItem, error)
	CreateOrganization(ctx context.Context, organization *domain.Organization) error
	AddMember(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, role domain.UserRole) error
	UpsertMember(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, role domain.UserRole) error
	ListMembers(ctx context.Context, organizationID uuid.UUID) ([]OrganizationMember, error)
	GetUserRoleInOrganization(ctx context.Context, userID, organizationID uuid.UUID) (role domain.UserRole, ok bool, err error)
	UpdateMemberRole(ctx context.Context, organizationID, targetUserID uuid.UUID, role domain.UserRole) error
	RemoveMember(ctx context.Context, organizationID, targetUserID uuid.UUID) error
	OrganizationCreatedBy(ctx context.Context, organizationID uuid.UUID) (uuid.UUID, error)
	// SeasonTargets is a JSON object (e.g. ndvi_target, health_score_target, notes); empty object if unset.
	GetSeasonTargets(ctx context.Context, organizationID uuid.UUID) (json.RawMessage, error)
	UpdateSeasonTargets(ctx context.Context, organizationID uuid.UUID, targets json.RawMessage) error
}

// FieldAuditEntry is one audit row for a field.
type FieldAuditEntry struct {
	ID          uuid.UUID
	FieldID     uuid.UUID
	ActorUserID uuid.UUID
	Action      string
	Payload     json.RawMessage
	CreatedAt   time.Time
}

type FieldAuditRepository interface {
	Insert(ctx context.Context, fieldID, actorUserID uuid.UUID, action string, payload json.RawMessage) error
	ListByFieldID(ctx context.Context, fieldID uuid.UUID, limit int) ([]FieldAuditEntry, error)
	ListByOrganizationID(ctx context.Context, organizationID uuid.UUID, limit int) ([]FieldAuditOrgEntry, error)
}

// FieldAuditOrgEntry is an audit row with field name for org-wide feeds.
type FieldAuditOrgEntry struct {
	FieldAuditEntry
	FieldName string
}

// OrgDashboardRepository aggregates SQL for the organization home dashboard.
type OrgDashboardRepository interface {
	Stats(ctx context.Context, organizationID uuid.UUID) (OrgDashboardStats, error)
	ObservedNdviWeekly(ctx context.Context, organizationID uuid.UUID) ([]OrgNdviWeekPoint, error)
	StaleFields(ctx context.Context, organizationID uuid.UUID, limit int32) ([]OrgStaleFieldRow, error)
	FieldsCentroidWGS84(ctx context.Context, organizationID uuid.UUID) (lon, lat *float64, err error)
}

type OrgDashboardStats struct {
	FieldCount                  int32
	TotalAreaHa                 float64
	FieldsWithObservedAnalytics int32
	MemberCount                 int32
}

type OrgNdviWeekPoint struct {
	WeekStart   time.Time
	NdviMeanAvg float64
}

type OrgStaleFieldRow struct {
	FieldID         uuid.UUID
	Name            string
	LastAnalyticsAt *time.Time
	TileCount       int32
}

// UserPreferences is persisted UI / profile settings (IdP fields stay in JWT).
type UserPreferences struct {
	UserID            uuid.UUID
	Locale            string
	Timezone          string
	AvatarURL         string
	UnitsSystem       string
	DateFormat        string
	FieldsDefaultYear *int32
	UpdatedAt         time.Time
}

type UserPreferencesRepository interface {
	Get(ctx context.Context, userID uuid.UUID) (*UserPreferences, error)
	Upsert(ctx context.Context, p *UserPreferences) error
}

// UserRepository resolves users (e.g. by email for invites) and creates them on first login.
type UserRepository interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) error
	// Upsert inserts or updates by primary key (first login / concurrent requests).
	Upsert(ctx context.Context, user *domain.User) error
}

// TileInfo is a tile identity and geometry for sending to geo-worker.
type TileInfo struct {
	ID          uuid.UUID
	GeometryWkb []byte
}

type TileRepository interface {
	CreateTile(ctx context.Context, fieldID uuid.UUID, geometryWkb []byte) (uuid.UUID, error)
	DeleteTilesByFieldID(ctx context.Context, fieldID uuid.UUID) error
	InsertTileWithID(ctx context.Context, tileID, fieldID uuid.UUID, geometryWkb []byte) error
	ListTilesByFieldID(ctx context.Context, fieldID uuid.UUID) ([]TileInfo, error)
	ListTileIDsByFieldID(ctx context.Context, fieldID uuid.UUID) ([]uuid.UUID, error)
	ListTilesGeoJSONByFieldID(ctx context.Context, fieldID uuid.UUID) ([]TileGeoJSONRow, error)
	GetFieldIDByTileID(ctx context.Context, tileID uuid.UUID) (uuid.UUID, error)
}

type TileGeoJSONRow struct {
	ID           uuid.UUID
	GeometryJSON []byte
}

// TileTimeseriesRow is one observation row for tile_timeseries.
type TileTimeseriesRow struct {
	TileID             uuid.UUID
	ObservationDate    time.Time
	Vh, Vv, Nbr2, Ndmi, Ndre, Ndvi, Gndvi, Msavi                 *float64
	DryDays            *int32
	BareSoilIndex, ValidPixelRatio                               *float64
	TemperatureCMean, PrecipitationMm3d, PrecipitationMm7d, PrecipitationMm30d *float64
}

type TileTimeseriesRepository interface {
	GetMaxObservationDateForField(ctx context.Context, fieldID uuid.UUID) (*time.Time, error)
	InsertTileTimeseries(ctx context.Context, row *TileTimeseriesRow) error
	ListByTileID(ctx context.Context, tileID uuid.UUID) ([]TileTimeseriesRow, error)
}

type SeasonRepository interface {
	CreateSeason(ctx context.Context, fieldID uuid.UUID, name string, startDate, endDate time.Time, isAuto bool) (uuid.UUID, error)
	GetSeasonByID(ctx context.Context, id uuid.UUID) (*domain.Season, error)
	ListSeasonsByFieldID(ctx context.Context, fieldID uuid.UUID) ([]domain.Season, error)
	UpdateSeason(ctx context.Context, id uuid.UUID, name string, startDate, endDate time.Time, isAuto bool) error
	DeleteSeason(ctx context.Context, id uuid.UUID) error
}

type Transactor interface {
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// TileMetricsReader returns observed timeseries and ML predictions for a tile (for tooltip).
type TileMetricsReader interface {
	GetTileMetrics(ctx context.Context, tileID uuid.UUID) (*TileMetrics, error)
}

// TileMetrics is the response for GET /tiles/{id}/metrics.
type TileMetrics struct {
	TileID     uuid.UUID
	Observed   []TileObservedRow
	Predictions []TilePredictionRow
}

type TileObservedRow struct {
	ObservationDate        time.Time
	Ndvi, Ndmi, Ndre       *float64
	ValidPixelRatio        *float64
	StressIndex            *float64
	TemperatureCMean       *float64
	PrecipitationMm3d, Mm7d, Mm30d *float64
}

type TilePredictionRow struct {
	ID             uuid.UUID
	Module         string
	PredictionDate time.Time
	Status         string
	// One of the below set based on Module
	Degradation   *TilePredictionDegradationDetail
	HealthStress  *TilePredictionHealthStressDetail
	Irrigation    *TilePredictionIrrigationDetail
}

type TilePredictionDegradationDetail struct {
	DegradationScore         *float64
	DegradationLevel         string
	Trend                    string
	VegetationCoverLossScore *float64
	BareSoilExpansionScore   *float64
	HeterogeneityScore       *float64
	AlertLevel               string
}

type TilePredictionHealthStressDetail struct {
	HealthScore            *float64
	StressScoreTotal       *float64
	WaterStress            *float64
	VegetationActivityDrop *float64
	HeterogeneityGrowth    *float64
	AlertLevel             string
	Trend                  string
}

type TilePredictionIrrigationDetail struct {
	IsIrrigated              *bool
	Confidence               *float64
	WaterBalanceStatus       string
	UnderIrrigationRiskScore *float64
	OverIrrigationRiskScore *float64
	UniformityScore          *float64
}

// MLResultStore persists ML module completion payloads into tile_predictions tables.
type MLResultStore interface {
	SaveMLCompletion(ctx context.Context, body []byte) error
}

// FieldAnalyticsRepository aggregates tile_timeseries into field_analytics_timeseries.
type FieldAnalyticsRepository interface {
	UpsertFieldAnalyticsForFieldAndDate(ctx context.Context, fieldID uuid.UUID, observationDate time.Time) error
	UpsertFieldPredictedAnalyticsForFieldAndDate(ctx context.Context, fieldID uuid.UUID, observationDate time.Time, m PredictedFieldAnalyticsMeans) error
	DeletePredictedFieldAnalyticsByFieldID(ctx context.Context, fieldID uuid.UUID) error
	ListFieldAnalyticsByFieldID(ctx context.Context, fieldID uuid.UUID, dateFrom, dateTo *time.Time) ([]FieldAnalyticsRow, error)
	DeleteFieldAnalyticsByDates(ctx context.Context, fieldID uuid.UUID, dates []time.Time) error
}

// PredictedFieldAnalyticsMeans holds field-level averages of ML outputs (finalize activity).
type PredictedFieldAnalyticsMeans struct {
	TileCount                 int32
	DegradationScore          *float64
	HealthScore               *float64
	StressScoreTotal          *float64
	WaterStress               *float64
	VegetationActivityDrop    *float64
	HeterogeneityGrowth       *float64
	Confidence                *float64
	IrrigationEventsDetected  *float64
}

// FieldAnalyticsRow is one row from field_analytics_timeseries for API.
type FieldAnalyticsRow struct {
	ID                     uuid.UUID
	FieldID                uuid.UUID
	ObservationDate        time.Time
	Source                 string
	TileCount              *int32
	ValidTileCount         *int32
	NdviMean               *float64
	NdmiMean               *float64
	NdreMean               *float64
	GndviMean              *float64
	MsaviMean              *float64
	Nbr2Mean               *float64
	BareSoilIndexMean      *float64
	ValidPixelRatioMean    *float64
	StressIndexMean        *float64
	TemperatureCMean       *float64
	PrecipitationMm3dMean  *float64
	PrecipitationMm7dMean  *float64
	PrecipitationMm30dMean *float64
	HeterogeneityScore     *float64
	// ML / predicted aggregates (typically source=predicted).
	PredictionDegradationScore         *float64
	PredictionVegetationCoverLossScore *float64
	PredictionBareSoilExpansionScore   *float64
	PredictionHealthScore              *float64
	PredictionStressScoreTotal         *float64
	PredictionWaterStress              *float64
	PredictionVegetationActivityDrop   *float64
	PredictionHeterogeneityGrowth      *float64
	PredictionConfidence               *float64
	PredictionIrrigationEventsDetected *float64
	PredictionUnderIrrigationRiskScore *float64
	PredictionOverIrrigationRiskScore  *float64
	PredictionUniformityScore          *float64
	CreatedAt                          time.Time
}

// AnalysisPmtilesRepository reads/writes PMTiles artifact URLs.
type AnalysisPmtilesRepository interface {
	ListByFieldID(ctx context.Context, fieldID uuid.UUID) ([]PmtilesArtifactRow, error)
	UpsertArtifact(ctx context.Context, fieldID uuid.UUID, analysisKind string, analysisDate time.Time, module, pmtilesURL string) error
	DeleteArtifactsByDates(ctx context.Context, fieldID uuid.UUID, dates []time.Time) error
	DeletePredictionArtifactsByFieldID(ctx context.Context, fieldID uuid.UUID) error
}

// PmtilesArtifactRow is one row from analysis_pmtiles_artifacts for API.
type PmtilesArtifactRow struct {
	ID           uuid.UUID
	FieldID      uuid.UUID
	AnalysisKind string
	AnalysisDate time.Time
	Module       string
	PmtilesUrl   string
	CreatedAt    time.Time
}


package ports

import (
	"context"
	"database/sql"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/app/domain"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/outbox"
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
	ID             uuid.UUID
	Name           string
	Description    string
	CreatedAt      time.Time
	AreaHectares   *float64
	OrganizationID uuid.UUID
	Coordinates    domain.Polygon
}

// OrganizationListItem is a row for list APIs.
type OrganizationListItem struct {
	ID   uuid.UUID
	Name string
}

type OrganizationRepository interface {
	ListForUser(ctx context.Context, userID uuid.UUID) ([]OrganizationListItem, error)
	CreateOrganization(ctx context.Context, organization *domain.Organization) error
	AddMember(ctx context.Context, organizationID uuid.UUID, userID uuid.UUID, role domain.UserRole) error
}

// UserRepository resolves users (e.g. by email for invites) and creates them on first login.
type UserRepository interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	Create(ctx context.Context, user *domain.User) error
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

type Publisher interface {
	Publish(ctx context.Context, message outbox.OutboxEvent) error
}

// TilesPublisher sends a tile-job request (field_id + field_geom GeoJSON) to the tile-worker.
type TilesPublisher interface {
	PublishTileRequest(ctx context.Context, body []byte, replyTo, correlationID string) error
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
	ListFieldAnalyticsByFieldID(ctx context.Context, fieldID uuid.UUID, dateFrom, dateTo *time.Time) ([]FieldAnalyticsRow, error)
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
	ValidPixelRatioMean    *float64
	StressIndexMean        *float64
	TemperatureCMean       *float64
	PrecipitationMm3dMean  *float64
	PrecipitationMm7dMean  *float64
	PrecipitationMm30dMean  *float64
	CreatedAt              time.Time
}

// AnalysisPmtilesRepository reads/writes PMTiles artifact URLs.
type AnalysisPmtilesRepository interface {
	ListByFieldID(ctx context.Context, fieldID uuid.UUID) ([]PmtilesArtifactRow, error)
	UpsertArtifact(ctx context.Context, fieldID uuid.UUID, analysisKind string, analysisDate time.Time, module, pmtilesURL string) error
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

// PmtilesBuildPublisher sends "build PMTiles" job to the pmtiles-worker queue.
type PmtilesBuildPublisher interface {
	PublishBuildJob(ctx context.Context, fieldID uuid.UUID, analysisKind string, analysisDate time.Time, module string) error
}

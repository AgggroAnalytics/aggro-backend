package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/http_adapter"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/temporalworkflows"
	fieldusecase "github.com/AgggroAnalytics/aggro-backend/internal/app/usecase/field"
	"github.com/AgggroAnalytics/aggro-backend/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/client"
)

func main() {
	cfg := configFromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := connectDBWithRetry(ctx, cfg.DatabaseURL, 30, 2*time.Second)
	if err != nil {
		slog.Error("db connect", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := migrate.Run(cfg.DatabaseURL); err != nil {
		slog.Error("migrate", "err", err)
		os.Exit(1)
	}

	fieldsRepo := postgres.NewFieldsPostgres(pool)
	tileRepo := postgres.NewTilesPostgres(pool)
	tileTsRepo := postgres.NewTileTimeseriesPostgres(pool)
	seasonRepo := postgres.NewSeasonsPostgres(pool)
	fieldAnalyticsRepo := postgres.NewFieldAnalyticsPostgres(pool)
	organizationRepo := postgres.NewOrganizationsPostgres(pool)
	orgDashboardRepo := postgres.NewOrgDashboardPostgres(pool)
	userRepo := postgres.NewUsersPostgres(pool)
	fieldAuditRepo := postgres.NewFieldAuditPostgres(pool)
	userPrefsRepo := postgres.NewUserPreferencesPostgres(pool)
	tileMetricsReader := postgres.NewTileMetricsPostgres(pool)

	fieldUC := fieldusecase.NewFieldsUseCase(fieldsRepo, seasonRepo)
	pmtilesRepo := postgres.NewAnalysisPmtilesPostgres(pool)

	deps := &httpadapter.RouterDeps{
		FieldUC:              fieldUC,
		FieldRepo:            fieldsRepo,
		SeasonRepo:           seasonRepo,
		FieldAnalyticsRepo:   fieldAnalyticsRepo,
		PmtilesRepo:          pmtilesRepo,
		TileRepo:             tileRepo,
		OrganizationRepo:     organizationRepo,
		OrgDashboard:         orgDashboardRepo,
		UserRepo:             userRepo,
		FieldAuditRepo:       fieldAuditRepo,
		UserPrefsRepo:        userPrefsRepo,
		TileMetricsReader:    tileMetricsReader,
		TileTsRepo:           tileTsRepo,
		S3Client:             httpadapter.NewS3Client(cfg.S3InternalURL, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Region),
		S3Bucket:             cfg.S3Bucket,
		TemporalNamespace:    cfg.TemporalNamespace,
		TemporalTaskQueue:    cfg.TemporalTaskQueue,
		TemporalBackendQueue: cfg.TemporalBackendTaskQueue,
	}

	if cfg.TemporalAddress != "" {
		go func() {
			for {
				if ctx.Err() != nil {
					return
				}
				tc, terr := client.Dial(client.Options{
					HostPort:  cfg.TemporalAddress,
					Namespace: cfg.TemporalNamespace,
				})
				if terr != nil {
					slog.Warn("temporal client not ready, retrying in 5s", "err", terr)
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
					}
					continue
				}
				deps.TemporalFieldWorkflows = &temporalworkflows.Lister{Client: tc, Namespace: cfg.TemporalNamespace}
				deps.TemporalClient = tc
				slog.Info("temporal client connected", "addr", cfg.TemporalAddress)
				return
			}
		}()
	}

	router := httpadapter.NewRouter(deps)
	handler := http.Handler(router)
	if cfg.KeycloakIssuer != "" {
		handler = httpadapter.AuthMiddleware(cfg.KeycloakIssuer, cfg.KeycloakJWKSURI, httpadapter.EnsureUserMiddleware(userRepo, router))
	}
	corsOrigins := cfg.CORSAllowOrigins
	if corsOrigins == "" {
		corsOrigins = "http://localhost:5173,http://127.0.0.1:5173,http://[::1]:5173"
	}
	handler = httpadapter.CORS(corsOrigins, handler)

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: handler}
	go func() {
		slog.Info("http listen", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http serve", "err", err)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	_ = srv.Shutdown(context.Background())
}

type config struct {
	DatabaseURL              string
	HTTPAddr                 string
	KeycloakIssuer           string
	KeycloakJWKSURI          string
	S3InternalURL            string
	S3Bucket                 string
	S3AccessKey              string
	S3SecretKey              string
	S3Region                 string
	TemporalAddress          string
	TemporalNamespace        string
	TemporalTaskQueue        string
	TemporalBackendTaskQueue string
	CORSAllowOrigins         string
}

func configFromEnv() config {
	return config{
		DatabaseURL:              getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/aggro"),
		HTTPAddr:                 getEnv("HTTP_ADDR", ":8080"),
		KeycloakIssuer:           getEnv("KEYCLOAK_ISSUER", ""),
		KeycloakJWKSURI:          getEnv("KEYCLOAK_JWKS_URI", ""),
		S3InternalURL:            getEnv("S3_INTERNAL_URL", "http://localhost:9000"),
		S3Bucket:                 getEnv("S3_BUCKET", "aggro"),
		S3AccessKey:              getEnv("S3_ACCESS_KEY_ID", "minioadmin"),
		S3SecretKey:              getEnv("S3_SECRET_ACCESS_KEY", "minioadmin"),
		S3Region:                 getEnv("S3_REGION", "us-east-1"),
		TemporalAddress:          getEnv("TEMPORAL_ADDRESS", ""),
		TemporalNamespace:        getEnv("TEMPORAL_NAMESPACE", "default"),
		TemporalTaskQueue:        getEnv("TEMPORAL_TASK_QUEUE", "field-processing"),
		TemporalBackendTaskQueue: getEnv("TEMPORAL_BACKEND_TASK_QUEUE", "aggro-backend"),
		CORSAllowOrigins:         getEnv("CORS_ALLOW_ORIGINS", ""),
	}
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func connectDBWithRetry(ctx context.Context, databaseURL string, attempts int, interval time.Duration) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		pool, err := pgxpool.New(ctx, databaseURL)
		if err == nil {
			return pool, nil
		}
		lastErr = err
		if i < attempts-1 {
			slog.Warn("db connect retry", "attempt", i+1, "err", err)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return nil, lastErr
}

// Temporal worker for backend-owned activities (e.g. finalize_field_processing_db).
// Run alongside aggro-field-worker: this process listens on TEMPORAL_BACKEND_TASK_QUEUE (default aggro-backend).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres"
	temporalactivities "github.com/AgggroAnalytics/aggro-backend/internal/temporal/activities"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/usecase/workflowfinalize"
	"github.com/AgggroAnalytics/aggro-backend/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
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
	fieldAnalyticsRepo := postgres.NewFieldAnalyticsPostgres(pool)
	mlResultStore := postgres.NewTilePredictionPostgres(pool)
	pmtilesRepo := postgres.NewAnalysisPmtilesPostgres(pool)
	transactor := postgres.NewTransactor(pool)

	deps := workflowfinalize.Deps{
		Transactor:         transactor,
		FieldRepo:          fieldsRepo,
		TileRepo:           tileRepo,
		TileTsRepo:         tileTsRepo,
		FieldAnalyticsRepo: fieldAnalyticsRepo,
		MLResultStore:      mlResultStore,
		PmtilesRepo:        pmtilesRepo,
	}
	finalize := &temporalactivities.Finalize{Deps: deps}

	c, err := client.DialContext(ctx, client.Options{
		HostPort:  cfg.TemporalAddress,
		Namespace: cfg.TemporalNamespace,
	})
	if err != nil {
		slog.Error("temporal client", "err", err)
		os.Exit(1)
	}
	defer c.Close()

	w := worker.New(c, cfg.TaskQueue, worker.Options{})
	w.RegisterActivityWithOptions(
		finalize.FinalizeFieldProcessingDB,
		activity.RegisterOptions{Name: "finalize_field_processing_db"},
	)

	slog.Info("temporal backend worker", "address", cfg.TemporalAddress, "namespace", cfg.TemporalNamespace, "task_queue", cfg.TaskQueue)
	if err := w.Run(worker.InterruptCh()); err != nil {
		slog.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}

type config struct {
	DatabaseURL       string
	TemporalAddress   string
	TemporalNamespace string
	TaskQueue         string
}

func configFromEnv() config {
	return config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/aggro"),
		TemporalAddress:   getEnv("TEMPORAL_ADDRESS", "localhost:7233"),
		TemporalNamespace: getEnv("TEMPORAL_NAMESPACE", "default"),
		TaskQueue:         getEnv("TEMPORAL_BACKEND_TASK_QUEUE", "aggro-backend"),
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
			if pingErr := pool.Ping(ctx); pingErr == nil {
				return pool, nil
			} else {
				pool.Close()
				err = pingErr
			}
		}
		lastErr = err
		if i < attempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}
		}
	}
	return nil, lastErr
}

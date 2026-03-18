package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/broker/rabbitmq"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/http_adapter"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/postgres"
	"github.com/AgggroAnalytics/aggro-backend/internal/app/consumer"
	fieldusecase "github.com/AgggroAnalytics/aggro-backend/internal/app/usecase/field"
	"github.com/AgggroAnalytics/aggro-backend/internal/adapters/temporalworkflows"
	"github.com/AgggroAnalytics/aggro-backend/internal/migrate"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rabbitmq/amqp091-go"
	"go.temporal.io/sdk/client"
)

func main() {
	cfg := configFromEnv()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Retry DB and RabbitMQ so we tolerate pod startup order in k8s
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

	// Repos
	fieldsRepo := postgres.NewFieldsPostgres(pool)
	tileRepo := postgres.NewTilesPostgres(pool)
	tileTsRepo := postgres.NewTileTimeseriesPostgres(pool)
	seasonRepo := postgres.NewSeasonsPostgres(pool)
	fieldAnalyticsRepo := postgres.NewFieldAnalyticsPostgres(pool)
	mlResultStore := postgres.NewTilePredictionPostgres(pool)
	organizationRepo := postgres.NewOrganizationsPostgres(pool)
	userRepo := postgres.NewUsersPostgres(pool)
	tileMetricsReader := postgres.NewTileMetricsPostgres(pool)

	// RabbitMQ: use a channel holder so we can reconnect without changing publisher refs
	holder := rabbitmq.NewChannelHolder()
	tilesPub := rabbitmq.NewRabbitPublisher(holder, cfg.TilesExchange, cfg.TilesRouting)
	geoPub := rabbitmq.NewRabbitPublisher(holder, cfg.GeoExchange, cfg.GeoRouting)
	mlPub := rabbitmq.NewRabbitPublisher(holder, cfg.MLExchange, cfg.MLRouting)
	pmtilesBuildPub := rabbitmq.NewPmtilesBuildPublisherAdapter(
		rabbitmq.NewRabbitPublisher(holder, cfg.PmtilesExchange, cfg.PmtilesRouting),
	)

	// Reply handler: tile + geo + ML completion + pmtiles build trigger
	handlers := &consumer.ReplyHandlers{
		TileRepo:              tileRepo,
		TileTsRepo:            tileTsRepo,
		SeasonRepo:            seasonRepo,
		FieldAnalyticsRepo:    fieldAnalyticsRepo,
		GeoPub:                geoPub,
		MLResultStore:         mlResultStore,
		PmtilesBuildPublisher: pmtilesBuildPub,
		BackendReplyQueue:     cfg.BackendReplyQueue,
	}
	replyHandler := func(ctx context.Context, body []byte) error {
		return handlers.HandleReply(ctx, body)
	}

	// Reconnect loop: when the reply consumer dies (conn/channel closed), reconnect and restart
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			conn, err := connectRabbitWithRetry(cfg.RabbitURL, 30, 2*time.Second)
			if err != nil {
				slog.Error("rabbit connect", "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			ch, err := conn.Channel()
			if err != nil {
				conn.Close()
				slog.Error("rabbit channel", "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			holder.Set(conn, ch)
			if err := rabbitmq.DeclareTopology(ch, []string{
				cfg.TilesExchange,
				cfg.GeoExchange,
				cfg.MLExchange,
				cfg.PmtilesExchange,
			}, cfg.BackendReplyQueue); err != nil {
				ch.Close()
				conn.Close()
				holder.Set(nil, nil)
				slog.Error("declare topology", "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
				continue
			}
			rabbitmq.ListenReturns(ch)
			doneCh := make(chan struct{}, 1)
			go func() {
				err := rabbitmq.ConsumeReplies(ctx, conn, cfg.BackendReplyQueue, replyHandler)
				if err != nil && ctx.Err() == nil {
					slog.Error("consume replies", "err", err)
				}
				doneCh <- struct{}{}
			}()
			select {
			case <-ctx.Done():
				ch.Close()
				conn.Close()
				return
			case <-doneCh:
				holder.Set(nil, nil)
				ch.Close()
				conn.Close()
				slog.Error("reply consumer died, reconnecting in 5s")
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}
		}
	}()

	// Field usecase: create field -> DB + default season + publish to tile-worker
	fieldUC := fieldusecase.NewFieldsUseCase(fieldsRepo, seasonRepo, tilesPub, cfg.BackendReplyQueue)
	pmtilesRepo := postgres.NewAnalysisPmtilesPostgres(pool)

	var temporalFieldLister *temporalworkflows.Lister
	if cfg.TemporalAddress != "" {
		tc, terr := client.Dial(client.Options{
			HostPort:  cfg.TemporalAddress,
			Namespace: cfg.TemporalNamespace,
		})
		if terr != nil {
			slog.Warn("temporal client for /fields/{id}/workflows", "err", terr)
		} else {
			temporalFieldLister = &temporalworkflows.Lister{Client: tc, Namespace: cfg.TemporalNamespace}
		}
	}

	router := httpadapter.NewRouter(&httpadapter.RouterDeps{
		FieldUC:            fieldUC,
		FieldRepo:          fieldsRepo,
		SeasonRepo:         seasonRepo,
		FieldAnalyticsRepo: fieldAnalyticsRepo,
		PmtilesRepo:        pmtilesRepo,
		TileRepo:           tileRepo,
		PmtilesBuild:       pmtilesBuildPub,
		OrganizationRepo:   organizationRepo,
		UserRepo:           userRepo,
		TileMetricsReader:  tileMetricsReader,
		TileTsRepo:         tileTsRepo,
		MLPublisher:        mlPub,
		S3Client:               httpadapter.NewS3Client(cfg.S3InternalURL, cfg.S3AccessKey, cfg.S3SecretKey, cfg.S3Region),
		S3Bucket:               cfg.S3Bucket,
		TemporalFieldWorkflows: temporalFieldLister,
	})
	handler := http.Handler(router)
	if cfg.KeycloakIssuer != "" {
		handler = httpadapter.AuthMiddleware(cfg.KeycloakIssuer, cfg.KeycloakJWKSURI, httpadapter.EnsureUserMiddleware(userRepo, router))
	}
	// CORS must wrap auth so OPTIONS preflight is answered before JWT check.
	corsOrigins := cfg.CORSAllowOrigins
	if corsOrigins == "" && cfg.KeycloakIssuer != "" {
		corsOrigins = "http://localhost:5173,http://127.0.0.1:5173"
	}
	if corsOrigins != "" {
		handler = httpadapter.CORS(corsOrigins, handler)
	}

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
	DatabaseURL       string
	RabbitURL         string
	BackendReplyQueue string
	TilesExchange     string
	TilesRouting      string
	GeoExchange       string
	GeoRouting        string
	MLExchange        string
	MLRouting         string
	PmtilesExchange   string
	PmtilesRouting    string
	HTTPAddr          string
	KeycloakIssuer    string
	KeycloakJWKSURI   string
	S3InternalURL     string
	S3Bucket          string
	S3AccessKey       string
	S3SecretKey       string
	S3Region            string
	TemporalAddress     string
	TemporalNamespace   string
	CORSAllowOrigins    string
}

func configFromEnv() config {
	return config{
		DatabaseURL:       getEnv("DATABASE_URL", "postgresql://postgres:postgres@localhost:5432/aggro"),
		RabbitURL:         getEnv("RABBIT_URL", "amqp://guest:guest@localhost:5672/"),
		BackendReplyQueue: getEnv("BACKEND_REPLY_QUEUE", "field.backend.replies"),
		TilesExchange:     getEnv("TILES_EXCHANGE", "field.tiles.exchange"),
		TilesRouting:      getEnv("TILES_ROUTING", "field.tiles.main"),
		GeoExchange:       getEnv("GEO_EXCHANGE", "field.geo.exchange"),
		GeoRouting:        getEnv("GEO_ROUTING", "field.geo.main"),
		MLExchange:        getEnv("ML_EXCHANGE", "field.ml.exchange"),
		MLRouting:         getEnv("ML_ROUTING", "field.ml.main"),
		PmtilesExchange:   getEnv("PMTILES_EXCHANGE", "field.pmtiles.exchange"),
		PmtilesRouting:    getEnv("PMTILES_ROUTING", "field.pmtiles.build"),
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		KeycloakIssuer:    getEnv("KEYCLOAK_ISSUER", ""),
		KeycloakJWKSURI:   getEnv("KEYCLOAK_JWKS_URI", ""),
		S3InternalURL:     getEnv("S3_INTERNAL_URL", "http://localhost:9000"),
		S3Bucket:          getEnv("S3_BUCKET", "aggro"),
		S3AccessKey:       getEnv("S3_ACCESS_KEY_ID", "minioadmin"),
		S3SecretKey:       getEnv("S3_SECRET_ACCESS_KEY", "minioadmin"),
		S3Region:          getEnv("S3_REGION", "us-east-1"),
		TemporalAddress:   getEnv("TEMPORAL_ADDRESS", ""),
		TemporalNamespace: getEnv("TEMPORAL_NAMESPACE", "default"),
		CORSAllowOrigins:  getEnv("CORS_ALLOW_ORIGINS", ""),
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

func connectRabbitWithRetry(rabbitURL string, attempts int, interval time.Duration) (*amqp091.Connection, error) {
	var lastErr error
	for i := 0; i < attempts; i++ {
		conn, err := amqp091.Dial(rabbitURL)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if i < attempts-1 {
			slog.Warn("rabbit connect retry", "attempt", i+1, "err", err)
			time.Sleep(interval)
		}
	}
	return nil, lastErr
}

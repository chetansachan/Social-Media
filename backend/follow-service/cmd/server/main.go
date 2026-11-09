package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chetansachan/socialflow/follow-service/internal/config"
	"github.com/chetansachan/socialflow/follow-service/internal/database"
	"github.com/chetansachan/socialflow/follow-service/internal/events"
	"github.com/chetansachan/socialflow/follow-service/internal/handler"
	"github.com/chetansachan/socialflow/follow-service/internal/repository"
	"github.com/chetansachan/socialflow/follow-service/internal/services"
	"github.com/chetansachan/socialflow/platform/observability"
	followpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	followServiceName = "follow-service"
	grpcServiceName   = "follow.FollowService"

	shutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("follow-service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	logger := observability.NewLogger(followServiceName)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracing := observability.TracingConfigFromEnv(followServiceName)
	shutdownTracing, err := observability.SetupTracing(ctx, tracing)
	if err != nil {
		return fmt.Errorf("set up tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Warn("trace exporter shutdown failed", "error", err)
		}
	}()
	if tracing.Endpoint == "" {
		logger.Info("tracing export disabled; set OTEL_EXPORTER_OTLP_ENDPOINT to export spans")
	} else {
		logger.Info("tracing enabled", "endpoint", tracing.Endpoint, "sample_ratio", tracing.SampleRatio)
	}

	metrics := observability.NewMetrics(followServiceName)
	healthTracker := observability.NewHealth()

	port := cfg.Port
	if port == "" {
		port = "50053"
	}

	dbPool, err := database.Connect(database.Options{
		WriteURL: cfg.DatabaseUrl,
		ReadURL:  cfg.DatabaseReadUrl,
		MaxConns: cfg.DatabaseMaxConns,
		MinConns: cfg.DatabaseMinConns,
	})
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer dbPool.Close()

	healthTracker.AddCheck("postgres", func(ctx context.Context) error {
		return dbPool.Writer().Ping(ctx)
	})
	metrics.RegisterDBPool("writer", func() int32 {
		return dbPool.Writer().Stat().AcquiredConns()
	})
	if cfg.DatabaseReadUrl != "" {
		metrics.RegisterDBPool("reader", func() int32 {
			return dbPool.Reader().Stat().AcquiredConns()
		})
	}

	var redisClient *redis.Client
	client, err := database.ConnectRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Warn("redis unavailable; outbox events will accumulate until it recovers",
			"addr", cfg.RedisAddr, "error", err)
	} else {
		redisClient = client
		defer redisClient.Close()
		logger.Info("connected to redis", "addr", cfg.RedisAddr, "db", cfg.RedisDB)
	}

	followRepo := repository.NewFollowRepository(dbPool)
	followService := services.NewFollowService(followRepo)
	followHandler := handler.NewFollowHandler(followService)

	// -------------------------------------------------------------------------
	// Transactional outbox relay
	// -------------------------------------------------------------------------
	if redisClient != nil {
		outboxRepo := repository.NewOutboxRepository(dbPool)
		relay := events.NewOutboxRelay(
			outboxRepo,
			events.NewRedisPublisher(redisClient, cfg.EventStream),
			events.RelayOptions{
				PollInterval: cfg.OutboxPollInterval,
				BatchSize:    cfg.OutboxBatchSize,
				MaxAttempts:  cfg.OutboxMaxAttempts,
				StaleAfter:   cfg.OutboxStaleAfter,
				PruneAfter:   cfg.OutboxPruneAfter,
				ReclaimEvery: cfg.OutboxReclaimInterval,
				BaseBackoff:  cfg.OutboxBaseBackoff,
				MaxBackoff:   cfg.OutboxMaxBackoff,
				Logger:       slog.NewLogLogger(logger.Handler(), slog.LevelInfo),
			},
		)
		go relay.Run(ctx)
		logger.Info("publishing outbox events", "stream", cfg.EventStream)
	}

	// -------------------------------------------------------------------------
	// gRPC API
	// -------------------------------------------------------------------------
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(metrics.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(metrics.StreamServerInterceptor()),
	)
	followpb.RegisterFollowServiceServer(grpcServer, followHandler)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(grpcServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("listen on :%s: %w", port, err)
	}
	defer listener.Close()

	adminServer := &http.Server{
		Addr:              ":" + cfg.AdminPort,
		Handler:           healthTracker.AdminMux(metrics),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErrors := make(chan error, 2)
	go func() {
		logger.Info("grpc server listening", "addr", listener.Addr().String())
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErrors <- fmt.Errorf("serve grpc: %w", err)
		}
	}()
	go func() {
		logger.Info("admin server listening", "addr", adminServer.Addr,
			"endpoints", "/metrics,/healthz,/readyz")
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- fmt.Errorf("serve admin: %w", err)
		}
	}()

	select {
	case err := <-serveErrors:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received; draining connections")
	}

	healthTracker.SetReady(false)
	healthServer.SetServingStatus(grpcServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		logger.Warn("graceful stop timed out; forcing shutdown")
		grpcServer.Stop()
	}

	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down admin server: %w", err)
	}

	logger.Info("follow-service stopped")
	return nil
}

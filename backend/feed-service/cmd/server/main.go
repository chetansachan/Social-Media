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

	"github.com/chetansachan/socialflow/feed-service/internal/config"
	"github.com/chetansachan/socialflow/feed-service/internal/database"
	"github.com/chetansachan/socialflow/feed-service/internal/events"
	"github.com/chetansachan/socialflow/feed-service/internal/handler"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
	"github.com/chetansachan/socialflow/feed-service/internal/services"
	"github.com/chetansachan/socialflow/platform/observability"
	feedpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

const (
	feedServiceName = "feed-service"
	grpcServiceName = "feed.FeedService"

	shutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("feed-service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	logger := observability.NewLogger(feedServiceName)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracing := observability.TracingConfigFromEnv(feedServiceName)
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

	metrics := observability.NewMetrics(feedServiceName)
	healthTracker := observability.NewHealth()

	// Redis is an optimisation, not a hard dependency. If it is unavailable the
	// read path falls back to live pull aggregation.
	var redisClient *redis.Client
	client, err := database.Connect(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Warn("redis unavailable; running in pull-model fallback mode",
			"addr", cfg.RedisAddr, "error", err)
	} else {
		redisClient = client
		defer redisClient.Close()
		logger.Info("connected to redis", "addr", cfg.RedisAddr, "db", cfg.RedisDB)
	}

	followConn, err := dial(cfg.FollowServiceAddr)
	if err != nil {
		return fmt.Errorf("create follow-service client: %w", err)
	}
	defer followConn.Close()

	postConn, err := dial(cfg.PostServiceAddr)
	if err != nil {
		return fmt.Errorf("create post-service client: %w", err)
	}
	defer postConn.Close()

	feedRepo := repository.NewFeedRepository(
		feedpb.NewFollowServiceClient(followConn),
		feedpb.NewPostServiceClient(postConn),
		redisClient,
	)

	options := services.Options{
		CelebrityThreshold:   cfg.CelebrityThreshold,
		TimelineMaxSize:      cfg.TimelineMaxSize,
		TimelineTTL:          cfg.TimelineTTL,
		ActiveUserTTL:        time.Duration(cfg.InactiveThresholdDays) * 24 * time.Hour,
		CelebrityRecentLimit: 20,
	}

	fanoutService := services.NewFanoutService(feedRepo, options)
	feedService := services.NewFeedService(feedRepo, options)
	feedHandler := handler.NewFeedHandler(feedService)

	// The event consumer is the write-path bridge. It is optional so the
	// service can run read-only in development.
	if redisClient != nil && cfg.EventConsumerEnabled {
		consumer := events.NewConsumer(redisClient, fanoutService, cfg.EventStream, cfg.EventGroup, cfg.EventConsumerName)
		consumer.SetMetrics(metrics)
		go func() {
			if err := consumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("feed event consumer stopped", "error", err)
			}
		}()
		logger.Info("consuming domain events", "stream", cfg.EventStream, "group", cfg.EventGroup)
	}

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(metrics.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(metrics.StreamServerInterceptor()),
	)
	feedpb.RegisterFeedServiceServer(grpcServer, feedHandler)

	healthServer := health.NewServer()
	healthServer.SetServingStatus(grpcServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on :%s: %w", cfg.Port, err)
	}
	defer listener.Close()

	adminServer := &http.Server{
		Addr:              ":" + cfg.AdminPort,
		Handler:           healthTracker.AdminMux(metrics),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErrors := make(chan error, 2)
	go func() {
		logger.Info("grpc server listening", "addr", listener.Addr().String(),
			"celebrity_threshold", cfg.CelebrityThreshold,
			"timeline_max_size", cfg.TimelineMaxSize)
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

	logger.Info("feed-service stopped")
	return nil
}

// dial opens a lazy gRPC client whose spans and metadata continue the trace
// started at the gateway.
func dial(address string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
	)
}

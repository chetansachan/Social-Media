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

	"github.com/chetansachan/socialflow/chat-service/internal/config"
	"github.com/chetansachan/socialflow/chat-service/internal/database"
	"github.com/chetansachan/socialflow/chat-service/internal/handler"
	"github.com/chetansachan/socialflow/chat-service/internal/presence"
	"github.com/chetansachan/socialflow/chat-service/internal/realtime"
	"github.com/chetansachan/socialflow/chat-service/internal/repository"
	"github.com/chetansachan/socialflow/chat-service/internal/services"
	"github.com/chetansachan/socialflow/platform/observability"
	chatpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

const (
	chatServiceName = "chat-service"
	grpcServiceName = "chat.ChatService"

	shutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("chat-service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	logger := observability.NewLogger(chatServiceName)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracing := observability.TracingConfigFromEnv(chatServiceName)
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
	}

	metrics := observability.NewMetrics(chatServiceName)
	healthTracker := observability.NewHealth()

	port := cfg.Port
	if port == "" {
		port = "50057"
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

	redisClient := redis.NewClient(&redis.Options{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	defer redisClient.Close()

	redisCtx, cancelRedis := context.WithTimeout(ctx, 5*time.Second)
	redisUp := redisClient.Ping(redisCtx).Err() == nil
	cancelRedis()
	healthTracker.AddCheck("redis", func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	})
	if redisUp {
		logger.Info("connected to redis", "addr", cfg.RedisAddr)
	} else {
		logger.Warn("redis is unavailable; presence and typing degrade to offline")
	}

	var presenceStore *presence.Store
	var publisher *realtime.Publisher
	if redisUp {
		presenceStore = presence.NewStore(redisClient, cfg.PresenceTTL, cfg.TypingTTL)
		publisher = realtime.NewPublisher(redisClient, cfg.EventStream)
	}

	chatService := services.NewChatService(
		repository.NewChatRepository(dbPool),
		presenceStore,
		publisher,
	)
	chatHandler := handler.NewChatHandler(chatService)

	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(metrics.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(metrics.StreamServerInterceptor()),
	)
	chatpb.RegisterChatServiceServer(grpcServer, chatHandler)

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

	logger.Info("chat-service stopped")
	return nil
}

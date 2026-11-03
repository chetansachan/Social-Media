package main

import (
	"context"
	"errors"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/chetansachan/socialflow/follow-service/internal/config"
	"github.com/chetansachan/socialflow/follow-service/internal/database"
	"github.com/chetansachan/socialflow/follow-service/internal/events"
	"github.com/chetansachan/socialflow/follow-service/internal/handler"
	"github.com/chetansachan/socialflow/follow-service/internal/repository"
	"github.com/chetansachan/socialflow/follow-service/internal/services"
	followpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "follow-service ", log.LstdFlags|log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	port := cfg.Port
	if port == "" {
		port = "50053"
	}

	dbPool, err := database.Connect(cfg.DatabaseUrl)
	if err != nil {
		logger.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	var redisClient *redis.Client
	client, err := database.ConnectRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Printf("warning: Redis unavailable at %s (%v); outbox events will accumulate until it recovers", cfg.RedisAddr, err)
	} else {
		redisClient = client
		defer redisClient.Close()
		logger.Printf("connected to Redis at %s (db %d)", cfg.RedisAddr, cfg.RedisDB)
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
				Logger:       logger,
			},
		)
		go relay.Run(ctx)
		logger.Printf("publishing outbox events to stream %s", cfg.EventStream)
	}

	// -------------------------------------------------------------------------
	// gRPC API
	// -------------------------------------------------------------------------
	grpcServer := grpc.NewServer()
	followpb.RegisterFollowServiceServer(grpcServer, followHandler)

	healthServer := health.NewServer()
	healthServer.SetServingStatus("follow.FollowService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		logger.Fatalf("failed to listen: %v", err)
	}

	go func() {
		<-ctx.Done()
		logger.Printf("shutting down gracefully...")
		healthServer.SetServingStatus("follow.FollowService", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()
	}()

	logger.Printf("Follow Service running on port: %s", port)
	if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		logger.Fatalf("failed to serve: %v", err)
	}

	stop()
	logger.Printf("Follow Service stopped")
}

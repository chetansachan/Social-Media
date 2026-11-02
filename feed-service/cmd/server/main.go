package main

import (
	"context"
	"errors"
	"log"
	"net"
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
	feedpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.Load()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Redis is an optimisation, not a hard dependency. If it is unavailable the
	// read path falls back to live pull aggregation.
	var redisClient *redis.Client
	client, err := database.Connect(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Printf("warning: Redis unavailable at %s (%v); running in pull-model fallback mode", cfg.RedisAddr, err)
	} else {
		redisClient = client
		defer redisClient.Close()
		log.Printf("connected to Redis at %s (db %d)", cfg.RedisAddr, cfg.RedisDB)
	}

	followConn, err := grpc.NewClient(cfg.FollowServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to create follow-service client: %v", err)
	}
	defer followConn.Close()

	postConn, err := grpc.NewClient(cfg.PostServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to create post-service client: %v", err)
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
		go func() {
			if err := consumer.Run(ctx); err != nil {
				log.Printf("feed event consumer stopped: %v", err)
			}
		}()
		log.Printf("consuming events from stream %s (group %s)", cfg.EventStream, cfg.EventGroup)
	}

	grpcServer := grpc.NewServer()
	feedpb.RegisterFeedServiceServer(grpcServer, feedHandler)

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", cfg.Port, err)
	}

	go func() {
		<-ctx.Done()
		log.Println("shutting down Feed Service gracefully...")
		grpcServer.GracefulStop()
	}()

	log.Printf("Feed Service (hybrid fanout) running on :%s", cfg.Port)
	log.Printf("  Follow Service     : %s", cfg.FollowServiceAddr)
	log.Printf("  Post Service       : %s", cfg.PostServiceAddr)
	log.Printf("  Celebrity threshold : %d followers", cfg.CelebrityThreshold)
	log.Printf("  Timeline max size   : %d items", cfg.TimelineMaxSize)

	if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		log.Fatalf("failed to serve: %v", err)
	}

	log.Println("Feed Service stopped")
}

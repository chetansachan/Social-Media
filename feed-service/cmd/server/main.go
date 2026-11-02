package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/chetansachan/socialflow/feed-service/internal/config"
	"github.com/chetansachan/socialflow/feed-service/internal/database"
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

	// 1. Initialize Redis connection with resilient fallback
	var rdb *redis.Client
	redisClient, err := database.Connect(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		log.Printf("Warning: Redis unavailable at %s (%v). Operating in graceful pull-model fallback mode.\n", cfg.RedisAddr, err)
		rdb = nil
	} else {
		rdb = redisClient
		defer rdb.Close()
		log.Printf("Connected to Redis cache at %s (DB: %d)\n", cfg.RedisAddr, cfg.RedisDB)
	}

	// 2. Initialize gRPC connection to Follow Service
	followConn, err := grpc.NewClient(
		cfg.FollowServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to follow service: %v", err)
	}
	defer followConn.Close()
	followClient := feedpb.NewFollowServiceClient(followConn)

	// 3. Initialize gRPC connection to Post Service
	postConn, err := grpc.NewClient(
		cfg.PostServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to post service: %v", err)
	}
	defer postConn.Close()
	postClient := feedpb.NewPostServiceClient(postConn)

	// 4. Dependency Injection
	feedRepo := repository.NewFeedRepository(followClient, postClient, rdb)
	fanoutService := services.NewFanoutService(
		feedRepo,
		cfg.CelebrityThreshold,
		cfg.TimelineMaxSize,
		cfg.TimelineTTLHours,
	)
	feedService := services.NewFeedService(
		feedRepo,
		fanoutService,
		cfg.CelebrityThreshold,
		cfg.TimelineMaxSize,
		cfg.TimelineTTLHours,
	)
	feedHandler := handler.NewFeedHandler(feedService)

	// 5. Initialize gRPC Server
	grpcServer := grpc.NewServer()
	feedpb.RegisterFeedServiceServer(grpcServer, feedHandler)

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", cfg.Port, err)
	}

	// 6. Graceful Shutdown listener
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		log.Println("Shutting down Feed Service gracefully...")
		grpcServer.GracefulStop()
	}()

	log.Printf("Feed Service (Hybrid Model) running on :%s\n", cfg.Port)
	log.Printf("  Follow Service : %s\n", cfg.FollowServiceAddr)
	log.Printf("  Post Service   : %s\n", cfg.PostServiceAddr)
	log.Printf("  Celebrity Threshold : %d followers\n", cfg.CelebrityThreshold)
	log.Printf("  Timeline Max Size   : %d items\n", cfg.TimelineMaxSize)

	if err := grpcServer.Serve(listener); err != nil && err != grpc.ErrServerStopped {
		log.Fatalf("failed to serve: %v", err)
	}

	log.Println("Feed Service stopped")
}

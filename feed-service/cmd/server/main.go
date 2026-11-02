package main

import (
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/chetansachan/socialflow/feed-service/internal/config"
	"github.com/chetansachan/socialflow/feed-service/internal/handler"
	"github.com/chetansachan/socialflow/feed-service/internal/repository"
	"github.com/chetansachan/socialflow/feed-service/internal/services"
	feedpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	cfg := config.Load()

	// 1. Initialize gRPC connection to Follow Service
	followConn, err := grpc.NewClient(
		cfg.FollowServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to follow service: %v", err)
	}
	defer followConn.Close()
	followClient := feedpb.NewFollowServiceClient(followConn)

	// 2. Initialize gRPC connection to Post Service
	postConn, err := grpc.NewClient(
		cfg.PostServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to post service: %v", err)
	}
	defer postConn.Close()
	postClient := feedpb.NewPostServiceClient(postConn)

	// 3. Dependency Injection
	feedRepo := repository.NewFeedRepository(followClient, postClient)
	feedService := services.NewFeedService(feedRepo)
	feedHandler := handler.NewFeedHandler(feedService)

	// 4. Initialize gRPC Server
	grpcServer := grpc.NewServer()
	feedpb.RegisterFeedServiceServer(grpcServer, feedHandler)

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", cfg.Port, err)
	}

	// 5. Graceful Shutdown listener
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		log.Println("Shutting down Feed Service gracefully...")
		grpcServer.GracefulStop()
	}()

	log.Printf("Feed Service (MVP Pull-Model) running on :%s\n", cfg.Port)
	log.Printf("Connected to Follow Service at %s\n", cfg.FollowServiceAddr)
	log.Printf("Connected to Post Service at %s\n", cfg.PostServiceAddr)

	if err := grpcServer.Serve(listener); err != nil && err != grpc.ErrServerStopped {
		log.Fatalf("failed to serve: %v", err)
	}

	log.Println("Feed Service stopped")
}

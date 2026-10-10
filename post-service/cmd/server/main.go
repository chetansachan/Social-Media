package main

import (
	"log"
	"net"

	postpb "github.com/chetansachan/socialflow/proto"
	"github.com/chetansachan/socialflow/post-service/internal/config"
	"github.com/chetansachan/socialflow/post-service/internal/database"
	"github.com/chetansachan/socialflow/post-service/internal/handler"
	"github.com/chetansachan/socialflow/post-service/internal/repository"
	"github.com/chetansachan/socialflow/post-service/internal/services"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()
	port := cfg.Port
	if port == "" {
		port = "50052"
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	dbPool, err := database.Connect(cfg.DatabaseUrl)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	postRepo := repository.NewPostRepository(dbPool)
	postService := services.NewPostService(postRepo)
	postHandler := handler.NewPostHandler(postService)

	grpcServer := grpc.NewServer()
	postpb.RegisterPostServiceServer(grpcServer, postHandler)

	log.Printf("Post Service running on : %s\n", port)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

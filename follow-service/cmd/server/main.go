package main

import (
	"log"
	"net"

	"github.com/chetansachan/socialflow/follow-service/internal/config"
	"github.com/chetansachan/socialflow/follow-service/internal/database"
	"github.com/chetansachan/socialflow/follow-service/internal/handler"
	"github.com/chetansachan/socialflow/follow-service/internal/repository"
	"github.com/chetansachan/socialflow/follow-service/internal/services"
	followpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()
	port := cfg.Port
	if port == "" {
		port = "50053"
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

	followRepo := repository.NewFollowRepository(dbPool)
	followService := services.NewFollowService(followRepo)
	followHandler := handler.NewFollowHandler(followService)

	grpcServer := grpc.NewServer()
	followpb.RegisterFollowServiceServer(grpcServer, followHandler)

	log.Printf("Follow Service running on port: %s\n", port)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

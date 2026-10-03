package main

import (
	"log"
	"net"

	"google.golang.org/grpc"

	userpb "github.com/chetansachan/socialflow/proto"
	"github.com/chetansachan/socialflow/user-service/internal/config"
	"github.com/chetansachan/socialflow/user-service/internal/database"
	"github.com/chetansachan/socialflow/user-service/internal/handler"
	"github.com/chetansachan/socialflow/user-service/internal/repository"
	"github.com/chetansachan/socialflow/user-service/internal/service"
)

func main() {
	listener, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	cfg := config.Load()

	dbPool, err := database.Connect(cfg.DatabaseUrl)
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	userRepo := repository.NewUserRepository(dbPool)
	userService := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userService)

	grpcServer := grpc.NewServer()
	userpb.RegisterUserServiceServer(grpcServer, userHandler)

	log.Println("User Service running on :50051")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

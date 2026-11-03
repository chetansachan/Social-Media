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
	cfg := config.Load()
	port := cfg.Port
	if port == "" {
		port = "50051"
	}

	listener, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	dbPool, err := database.Connect(database.Options{
		WriteURL: cfg.DatabaseUrl,
		ReadURL:  cfg.DatabaseReadUrl,
		MaxConns: cfg.DatabaseMaxConns,
		MinConns: cfg.DatabaseMinConns,
	})
	if err != nil {
		log.Fatalf("failed to connect to database: %v", err)
	}
	defer dbPool.Close()

	userRepo := repository.NewUserRepository(dbPool)
	userService := service.NewUserService(userRepo)
	userHandler := handler.NewUserHandler(userService)

	grpcServer := grpc.NewServer()
	userpb.RegisterUserServiceServer(grpcServer, userHandler)

	log.Printf("User Service running on : %s\n", port)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

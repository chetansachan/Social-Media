package main

import (
	"context"
	"log"
	"net"

	"google.golang.org/grpc"

	userpb "github.com/chetansachan/socialflow/proto"
	"github.com/chetansachan/socialflow/user-service/internal/cache"
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

	// The profile cache is optional: if Redis is down at boot, the service
	// still starts and serves reads from PostgreSQL behind singleflight.
	serviceOptions := make([]service.Option, 0, 1)
	if redisClient, err := database.ConnectRedis(context.Background(), cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB); err != nil {
		log.Printf("warning: Redis unavailable at %s (%v); profile cache disabled", cfg.RedisAddr, err)
	} else {
		defer redisClient.Close()
		serviceOptions = append(serviceOptions, service.WithUserCache(cache.NewRedisUserCache(redisClient), cfg.UserCacheTTL))
		log.Printf("profile cache enabled at %s (ttl %s)", cfg.RedisAddr, cfg.UserCacheTTL)
	}

	userService := service.NewUserService(userRepo, serviceOptions...)
	userHandler := handler.NewUserHandler(userService)

	grpcServer := grpc.NewServer()
	userpb.RegisterUserServiceServer(grpcServer, userHandler)

	log.Printf("User Service running on : %s\n", port)

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

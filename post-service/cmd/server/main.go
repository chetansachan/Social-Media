package main

import (
	"fmt"
	"log"
	"net"

	"github.com/chetansachan/socialflow/post-service/internal/config"
	"google.golang.org/grpc"
)

func main() {
	cfg := config.Load()

	listener, err := net.Listen("tcp", ":" + cfg.Port)
	if err != nil {
		fmt.Printf("Unable to listen: %v", err)
		return
	}

	grpcServer := grpc.NewServer()

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

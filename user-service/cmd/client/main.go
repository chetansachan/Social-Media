package main

import (
	"context"
	"log"
	"time"

	userpb "github.com/chetansachan/socialflow/proto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	conn, err := grpc.NewClient(
		"localhost:50051",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	client := userpb.NewUserServiceClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	response, err := client.Register(ctx, &userpb.RegisterUserRequest{
		Username: "1",
	})
	if err != nil {
		log.Fatalf("failed to get user: %v", err)
	}

	if response.GetUser() != nil {
		log.Printf("User ID: %s", response.GetUser().GetId())
		log.Printf("User Name: %s", response.GetUser().GetName())
	}
}
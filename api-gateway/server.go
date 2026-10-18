package main

import (
	"log"
	"net/http"
	"os"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/chetansachan/socialflow/api-gateway/graph"
	userpb "github.com/chetansachan/socialflow/proto"
	"github.com/vektah/gqlparser/v2/ast"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultPort            = "8080"
	defaultUserServiceAddr = "localhost:50051"
	defaultPostServiceAddr = "localhost:50052"
	defaultFollowServiceAddr = "localhost:50053"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	userServiceAddr := os.Getenv("USER_SERVICE_ADDR")
	if userServiceAddr == "" {
		userServiceAddr = defaultUserServiceAddr
	}

	userConn, err := grpc.NewClient(
		userServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to user service: %v", err)
	}
	defer userConn.Close()

	userClient := userpb.NewUserServiceClient(userConn)

	postServiceAddr := os.Getenv("POST_SERVICE_ADDR")
	if postServiceAddr == "" {
		postServiceAddr = defaultPostServiceAddr
	}

	postConn, err := grpc.NewClient(
		postServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to post service: %v", err)
	}
	defer postConn.Close()

	postClient := userpb.NewPostServiceClient(postConn)

	followServiceAddr := os.Getenv("FOLLOW_SERVICE_ADDR")
	if followServiceAddr == "" {
		followServiceAddr = defaultFollowServiceAddr
	}

	followConn, err := grpc.NewClient(
		followServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to connect to follow service: %v", err)
	}
	defer followConn.Close()

	followClient := userpb.NewFollowServiceClient(followConn)

	srv := handler.New(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: &graph.Resolver{
					UserClient: userClient,
					PostClient: postClient,
					FollowClient: followClient,
				},
			},
		),
	)

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	http.Handle("/", playground.Handler("GraphQL playground", "/query"))
	http.Handle("/query", srv)

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

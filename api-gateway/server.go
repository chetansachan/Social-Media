package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/chetansachan/socialflow/api-gateway/graph"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/joho/godotenv"
	"github.com/vektah/gqlparser/v2/ast"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	defaultPort              = "8080"
	defaultUserServiceAddr   = "localhost:50051"
	defaultPostServiceAddr   = "localhost:50052"
	defaultFollowServiceAddr = "localhost:50053"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("no .env file found, using process environment")
	}

	port := envOr("PORT", defaultPort)
	secret := jwtSecret()

	userConn := dial(envOr("USER_SERVICE_ADDR", defaultUserServiceAddr))
	defer userConn.Close()
	postConn := dial(envOr("POST_SERVICE_ADDR", defaultPostServiceAddr))
	defer postConn.Close()
	followConn := dial(envOr("FOLLOW_SERVICE_ADDR", defaultFollowServiceAddr))
	defer followConn.Close()

	srv := handler.New(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers: &graph.Resolver{
					UserClient:   socialflowpb.NewUserServiceClient(userConn),
					PostClient:   socialflowpb.NewPostServiceClient(postConn),
					FollowClient: socialflowpb.NewFollowServiceClient(followConn),
				},
			},
		),
	)

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 30 * time.Second,
		InitFunc:              auth.WebsocketInit(secret),
	})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	mux := http.NewServeMux()
	mux.Handle("/", playground.Handler("GraphQL playground", "/query"))
	// Playground stays public; the @auth directive protects individual fields.
	mux.Handle("/query", auth.Middleware(secret, srv))

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	log.Printf("connect to http://localhost:%s/ for GraphQL playground", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("gateway stopped: %v", err)
	}
}

// dial opens a lazy gRPC client connection that forwards the authenticated
// caller to downstream services through metadata.
func dial(address string) *grpc.ClientConn {
	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(auth.UnaryClientInterceptor()),
	)
	if err != nil {
		log.Fatalf("failed to connect to %s: %v", address, err)
	}
	return conn
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

// jwtSecret must match the signing key used by user-service. JWT_SECRET wins
// so deployments can rotate the key without reusing the legacy MYSECRETKEY name.
func jwtSecret() string {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("MYSECRETKEY"))
	}
	if secret == "" {
		log.Fatal("JWT_SECRET or MYSECRETKEY must be set to validate access tokens")
	}
	return secret
}

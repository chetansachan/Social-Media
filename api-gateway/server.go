package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/chetansachan/socialflow/api-gateway/auth"
	"github.com/chetansachan/socialflow/api-gateway/graph"
	"github.com/chetansachan/socialflow/api-gateway/graph/dataloader"
	"github.com/chetansachan/socialflow/api-gateway/ratelimit"
	"github.com/chetansachan/socialflow/platform/observability"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
	"github.com/vektah/gqlparser/v2/ast"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"
)

const (
	defaultPort                    = "8080"
	defaultAdminPort               = "9090"
	defaultUserServiceAddr         = "localhost:50051"
	defaultPostServiceAddr         = "localhost:50052"
	defaultFollowServiceAddr       = "localhost:50053"
	defaultFeedServiceAddr         = "localhost:50054"
	defaultNotificationServiceAddr = "localhost:50055"
	defaultModerationServiceAddr   = "localhost:50056"
	defaultRedisAddr               = "localhost:6379"

	shutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if err := godotenv.Load(); err != nil {
		// Normal in production: configuration arrives as environment
		// variables from the orchestrator.
	}

	logger := observability.NewLogger("api-gateway")
	slog.SetDefault(logger)

	// SIGTERM is what Kubernetes sends before it stops routing traffic. The
	// context is cancelled first, then run drains the servers below.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracing := observability.TracingConfigFromEnv("api-gateway")
	shutdownTracing, err := observability.SetupTracing(ctx, tracing)
	if err != nil {
		return fmt.Errorf("set up tracing: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(shutdownCtx); err != nil {
			logger.Warn("trace exporter shutdown failed", "error", err)
		}
	}()
	if tracing.Endpoint == "" {
		logger.Info("tracing export disabled; set OTEL_EXPORTER_OTLP_ENDPOINT to export spans")
	} else {
		logger.Info("tracing enabled", "endpoint", tracing.Endpoint, "sample_ratio", tracing.SampleRatio)
	}

	metrics := observability.NewMetrics("api-gateway")
	health := observability.NewHealth()

	port := envOr("PORT", defaultPort)
	adminPort := envOr("ADMIN_PORT", defaultAdminPort)
	secret, err := jwtSecret()
	if err != nil {
		return err
	}

	connections, err := dialServices(logger)
	if err != nil {
		return err
	}
	defer func() {
		for _, conn := range connections {
			_ = conn.Close()
		}
	}()

	redisClient := redis.NewClient(&redis.Options{
		Addr:     envOr("REDIS_ADDR", defaultRedisAddr),
		Password: os.Getenv("REDIS_PASSWORD"),
		DB:       envInt("REDIS_DB", 0),
	})
	defer redisClient.Close()
	health.AddCheck("redis", func(ctx context.Context) error {
		return redisClient.Ping(ctx).Err()
	})

	pingCtx, cancelPing := context.WithTimeout(ctx, 5*time.Second)
	if err := redisClient.Ping(pingCtx).Err(); err != nil {
		// Revocation fails closed, so the gateway keeps running but stays
		// unready until Redis recovers instead of rejecting every session.
		logger.Warn("redis is not reachable; token revocation will fail closed", "error", err)
	} else {
		logger.Info("connected to redis", "addr", envOr("REDIS_ADDR", defaultRedisAddr))
	}
	cancelPing()

	revocations := auth.NewRedisRevocations(redisClient)

	// Field budgets are deliberately small for writes and generous for reads.
	// The guard keys on the root schema field, so a client cannot rename a
	// mutation to borrow a query's allowance.
	rateGuard := ratelimit.NewGuard(
		ratelimit.New(redisClient),
		map[string]ratelimit.Policy{
			"Login":         {Limit: envInt("RATE_LIMIT_LOGIN_PER_MINUTE", 5), Window: time.Minute},
			"register":      {Limit: envInt("RATE_LIMIT_REGISTER_PER_MINUTE", 5), Window: time.Minute},
			"CreatePost":    {Limit: envInt("RATE_LIMIT_CREATE_POST_PER_MINUTE", 10), Window: time.Minute},
			"CreateComment": {Limit: envInt("RATE_LIMIT_CREATE_COMMENT_PER_MINUTE", 30), Window: time.Minute},
			"getFeed":       {Limit: envInt("RATE_LIMIT_FEED_PER_MINUTE", 120), Window: time.Minute},
			// Search hits trigram and tsvector scans, so it gets a tighter
			// budget than the indexed feed read.
			"searchPosts": {Limit: envInt("RATE_LIMIT_SEARCH_PER_MINUTE", 60), Window: time.Minute},
			"searchUsers": {Limit: envInt("RATE_LIMIT_SEARCH_PER_MINUTE", 60), Window: time.Minute},
		},
		ratelimit.Policy{Limit: envInt("RATE_LIMIT_AUTHENTICATED_PER_MINUTE", 600), Window: time.Minute},
		ratelimit.Policy{Limit: envInt("RATE_LIMIT_ANONYMOUS_PER_MINUTE", 120), Window: time.Minute},
		logger,
	)

	userClient := socialflowpb.NewUserServiceClient(connections["user-service"])
	postClient := socialflowpb.NewPostServiceClient(connections["post-service"])
	// Comments are served by post-service on the same connection.
	commentClient := socialflowpb.NewCommentServiceClient(connections["post-service"])

	resolver := &graph.Resolver{
		UserClient:         userClient,
		PostClient:         postClient,
		CommentClient:      commentClient,
		FollowClient:       socialflowpb.NewFollowServiceClient(connections["follow-service"]),
		FeedClient:         socialflowpb.NewFeedServiceClient(connections["feed-service"]),
		NotificationClient: socialflowpb.NewNotificationServiceClient(connections["notification-service"]),
		ModerationClient:   socialflowpb.NewModerationServiceClient(connections["moderation-service"]),
		Redis:              redisClient,
		Revocations:        revocations,
		NewLoaders: func() *dataloader.Loaders {
			return dataloader.New(userClient, postClient, commentClient)
		},
	}

	srv := handler.New(
		graph.NewExecutableSchema(
			graph.Config{
				Resolvers:  resolver,
				Directives: graph.DirectiveRoot{Auth: auth.AuthDirective},
			},
		),
	)

	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.Websocket{
		KeepAlivePingInterval: 30 * time.Second,
		InitFunc:              auth.WebsocketInit(secret, revocations),
	})

	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))

	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})
	srv.Use(graphqlTracing{})
	srv.Use(rateGuard)

	mux := http.NewServeMux()
	mux.Handle("/", playground.Handler("GraphQL playground", "/query"))
	// Playground stays public; the @auth directive protects individual fields.
	// The loader middleware wraps the executing handler, not the playground.
	// Revocations run inside the auth middleware because they need the parsed
	// principal (and its jti) to decide whether the session was logged out.
	// The client-IP middleware sits outside auth so the rate-limit guard can
	// charge anonymous traffic even when no token was sent.
	//
	// Metrics wrap otelhttp so the status recorder also sees upgraded
	// connections and can skip their multi-minute durations.
	trustProxy := envBool("TRUST_PROXY_HEADERS", false)
	queryHandler := auth.Middleware(secret, auth.RejectRevoked(revocations, dataloader.Middleware(resolver.NewLoaders)(srv)))
	queryHandler = ratelimit.WithClientIP(trustProxy)(queryHandler)
	queryHandler = metrics.HTTPMiddleware(otelhttp.NewHandler(queryHandler, "graphql"))
	mux.Handle("/query", queryHandler)

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	adminServer := &http.Server{
		Addr:              ":" + adminPort,
		Handler:           health.AdminMux(metrics),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErrors := make(chan error, 2)
	go func() {
		logger.Info("graphql gateway listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("graphql server: %w", err)
		}
	}()
	go func() {
		logger.Info("admin server listening", "addr", adminServer.Addr,
			"endpoints", "/metrics,/healthz,/readyz")
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErrors <- fmt.Errorf("admin server: %w", err)
		}
	}()

	select {
	case err := <-serverErrors:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received; draining connections")
	}

	// Fail readiness first so a load balancer stops adding new requests while
	// the existing ones finish.
	health.SetReady(false)

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancelShutdown()

	var shutdownErr error
	if err := server.Shutdown(shutdownCtx); err != nil {
		shutdownErr = fmt.Errorf("shut down graphql server: %w", err)
	}
	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("shut down admin server: %w", err))
	}

	logger.Info("gateway stopped")
	return shutdownErr
}

// dialServices opens a small pool of lazy gRPC client connections per backend.
// otelgrpc injects the active trace context into metadata, and the auth
// interceptor forwards the authenticated caller, so downstream spans join the
// gateway's trace and downstream handlers can authorize without trusting
// client input.
func dialServices(logger *slog.Logger) (map[string]*connPool, error) {
	targets := []struct {
		name    string
		address string
	}{
		{"user-service", envOr("USER_SERVICE_ADDR", defaultUserServiceAddr)},
		{"post-service", envOr("POST_SERVICE_ADDR", defaultPostServiceAddr)},
		{"follow-service", envOr("FOLLOW_SERVICE_ADDR", defaultFollowServiceAddr)},
		{"feed-service", envOr("FEED_SERVICE_ADDR", defaultFeedServiceAddr)},
		{"notification-service", envOr("NOTIFICATION_SERVICE_ADDR", defaultNotificationServiceAddr)},
		{"moderation-service", envOr("MODERATION_SERVICE_ADDR", defaultModerationServiceAddr)},
	}

	connections := make(map[string]*connPool, len(targets))
	breakers := newBreakerSet(envDuration("GRPC_BREAKER_TIMEOUT", 10*time.Second))
	rpcTimeout := envDuration("GRPC_TIMEOUT", 3*time.Second)
	for _, target := range targets {
		pool, err := dial(target.address, rpcTimeout, breakers)
		if err != nil {
			for _, opened := range connections {
				_ = opened.Close()
			}
			return nil, fmt.Errorf("connect to %s at %s: %w", target.name, target.address, err)
		}
		connections[target.name] = pool
		logger.Info("registered gRPC target",
			"service", target.name,
			"address", target.address,
			"subchannels", len(pool.conns),
			"rpc_timeout", rpcTimeout)
	}
	return connections, nil
}

// dial opens a pool of lazy gRPC client connections. keepalive detects a dead
// backend between requests instead of waiting for the next caller to time out.
func dial(address string, rpcTimeout time.Duration, breakers *breakerSet) (*connPool, error) {
	subchannels := envInt("GRPC_SUBCHANNELS", 4)
	if subchannels < 1 {
		subchannels = 1
	}

	conns := make([]rpcConn, 0, subchannels)
	for i := 0; i < subchannels; i++ {
		conn, err := grpc.NewClient(
			address,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
			grpc.WithKeepaliveParams(keepalive.ClientParameters{
				Time:                30 * time.Second,
				Timeout:             10 * time.Second,
				PermitWithoutStream: true,
			}),
			// A headless Kubernetes service resolves to every pod address;
			// round robin spreads calls across them instead of pinning one
			// subchannel.
			grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
			grpc.WithChainUnaryInterceptor(
				auth.UnaryClientInterceptor(),
				timeoutInterceptor(rpcTimeout),
				breakers.Intercept,
			),
		)
		if err != nil {
			for _, opened := range conns {
				_ = opened.Close()
			}
			return nil, err
		}
		conns = append(conns, conn)
	}

	return newConnPool(conns...), nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return parsed
}

// jwtSecret must match the signing key used by user-service. JWT_SECRET wins
// so deployments can rotate the key without reusing the legacy MYSECRETKEY name.
func jwtSecret() (string, error) {
	secret := strings.TrimSpace(os.Getenv("JWT_SECRET"))
	if secret == "" {
		secret = strings.TrimSpace(os.Getenv("MYSECRETKEY"))
	}
	if secret == "" {
		return "", errors.New("JWT_SECRET or MYSECRETKEY must be set to validate access tokens")
	}
	return secret, nil
}

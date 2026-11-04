package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/chetansachan/socialflow/notification-service/internal/config"
	"github.com/chetansachan/socialflow/notification-service/internal/database"
	"github.com/chetansachan/socialflow/notification-service/internal/dispatchers"
	"github.com/chetansachan/socialflow/notification-service/internal/events"
	"github.com/chetansachan/socialflow/notification-service/internal/handler"
	"github.com/chetansachan/socialflow/notification-service/internal/repository"
	"github.com/chetansachan/socialflow/notification-service/internal/services"
	"github.com/chetansachan/socialflow/platform/observability"
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
)

const (
	notificationServiceName = "notification-service"
	grpcServiceName         = "notification.NotificationService"

	shutdownTimeout = 15 * time.Second
)

func main() {
	if err := run(); err != nil {
		slog.Error("notification-service stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := config.Load()
	logger := observability.NewLogger(notificationServiceName)
	slog.SetDefault(logger)

	// Most internal packages predate slog and accept *log.Logger. Route them
	// through the same JSON handler so every line has the same shape.
	stdLogger := slog.NewLogLogger(logger.Handler(), slog.LevelInfo)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	tracing := observability.TracingConfigFromEnv(notificationServiceName)
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

	metrics := observability.NewMetrics(notificationServiceName)
	healthTracker := observability.NewHealth()

	// PostgreSQL is the system of record: without it the inbox cannot serve
	// reads or persist events, so a failure here is fatal.
	dbPool, err := database.Connect(ctx, database.Options{
		WriteURL:        cfg.DatabaseURL,
		ReadURL:         cfg.DatabaseReadURL,
		MaxConns:        cfg.DBMaxOpenConns,
		MinConns:        cfg.DBMaxIdleConns,
		MaxConnLifetime: cfg.DBConnMaxLifetime,
	})
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer dbPool.Close()
	healthTracker.AddCheck("postgres", func(ctx context.Context) error {
		return dbPool.Writer().Ping(ctx)
	})
	metrics.RegisterDBPool("writer", func() int32 {
		return dbPool.Writer().Stat().AcquiredConns()
	})
	if cfg.DatabaseReadURL != "" {
		metrics.RegisterDBPool("reader", func() int32 {
			return dbPool.Reader().Stat().AcquiredConns()
		})
	}
	logger.Info("connected to postgresql")

	// Redis powers unread badges, dedup locks, aggregation windows, push rate
	// limits, and WebSocket fan-out. Every one of those degrades gracefully, so
	// the service still boots without Redis.
	var redisClient *redis.Client
	client, err := database.ConnectRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Warn("redis unavailable; running in degraded mode", "addr", cfg.RedisAddr, "error", err)
	} else {
		redisClient = client
		defer redisClient.Close()
		logger.Info("connected to redis", "addr", cfg.RedisAddr, "db", cfg.RedisDB)
	}

	// -------------------------------------------------------------------------
	// Repositories
	// -------------------------------------------------------------------------
	notificationRepo := repository.NewNotificationRepository(dbPool)
	settingsRepo := repository.NewSettingsRepository(dbPool, redisClient, 24*time.Hour)
	deviceRepo := repository.NewDeviceTokenRepository(dbPool)

	var cacheRepo repository.CacheRepository
	if redisClient != nil {
		cacheRepo = repository.NewCacheRepository(redisClient, cfg.UnreadCacheTTL)
	}

	// The User Service is only used to hydrate actor names/avatars. It is
	// optional: a missing profile must never hide a notification.
	var actorDirectory repository.ActorDirectory
	userConn, err := grpc.NewClient(
		cfg.UserServiceAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig":[{"round_robin":{}}]}`),
	)
	if err != nil {
		logger.Warn("failed to create user-service client", "error", err)
	} else {
		defer userConn.Close()
		actorDirectory = repository.NewActorDirectory(socialflowpb.NewUserServiceClient(userConn))
	}

	// -------------------------------------------------------------------------
	// Dispatchers
	// -------------------------------------------------------------------------
	realtime := dispatchers.NewRealtimeDispatcher(redisClient, stdLogger)
	realtime.SetMetrics(metrics)

	pushProvider, pushProviderName := buildPushProvider(cfg, stdLogger)
	pushDispatcher := dispatchers.NewPushDispatcher(deviceRepo, pushProvider, cfg.WorkerCount, cfg.QueueSize, stdLogger)
	pushDispatcher.SetMetrics(metrics)
	pushDispatcher.Start(ctx)

	emailProvider, emailProviderName := buildEmailProvider(cfg, stdLogger)
	emailDispatcher := dispatchers.NewEmailDispatcher(emailProvider, 2, cfg.QueueSize/4, stdLogger)
	emailDispatcher.SetMetrics(metrics)
	emailDispatcher.Start(ctx)

	// -------------------------------------------------------------------------
	// Domain services
	// -------------------------------------------------------------------------
	options := services.Options{
		AggregationWindow: cfg.AggregationWindow,
		DedupTTL:          cfg.DedupTTL,
		RealtimeThrottle:  cfg.RealtimeThrottle,
		MaxPushPerHour:    cfg.MaxPushPerHour,
		PushRateWindow:    cfg.PushRateWindow,
	}

	notificationService := services.NewNotificationService(services.Dependencies{
		Notifications: notificationRepo,
		Settings:      settingsRepo,
		Cache:         cacheRepo,
		Devices:       deviceRepo,
		Actors:        actorDirectory,
		Realtime:      realtime,
		Push:          pushDispatcher,
		Email:         emailDispatcher,
		Options:       options,
		Logger:        stdLogger,
	})

	// -------------------------------------------------------------------------
	// Event ingestion
	// -------------------------------------------------------------------------
	if redisClient != nil && cfg.EventConsumerEnabled {
		eventHandler := events.NewEventHandler(notificationService, stdLogger)
		consumer := events.NewConsumer(redisClient, eventHandler, events.Options{
			Streams:       cfg.EventStreams,
			Group:         cfg.EventGroup,
			Consumer:      cfg.EventConsumerName,
			DLQStream:     cfg.EventDLQStream,
			ClaimInterval: cfg.EventClaimInterval,
			MaxAttempts:   cfg.EventMaxAttempts,
			Logger:        stdLogger,
		})
		go func() {
			if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Error("event consumer stopped", "error", err)
			}
		}()
		logger.Info("consuming domain events",
			"streams", cfg.EventStreams, "group", cfg.EventGroup, "consumer", cfg.EventConsumerName)
	} else if cfg.EventConsumerEnabled {
		logger.Warn("event consumer disabled: redis is unavailable")
	}

	// -------------------------------------------------------------------------
	// Partition maintenance and retention
	// -------------------------------------------------------------------------
	maintainPartitions(ctx, stdLogger, notificationRepo, cfg.RetentionDays)

	// -------------------------------------------------------------------------
	// gRPC API
	// -------------------------------------------------------------------------
	grpcServer := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(metrics.UnaryServerInterceptor()),
		grpc.ChainStreamInterceptor(metrics.StreamServerInterceptor()),
	)
	socialflowpb.RegisterNotificationServiceServer(grpcServer, handler.NewGRPCHandler(notificationService, stdLogger))

	healthServer := health.NewServer()
	healthServer.SetServingStatus(grpcServiceName, healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		return fmt.Errorf("listen on grpc port %s: %w", cfg.Port, err)
	}
	defer listener.Close()

	// -------------------------------------------------------------------------
	// WebSocket gateway
	// -------------------------------------------------------------------------
	hub := handler.NewHub(ctx, redisClient, stdLogger)
	hub.SetMetrics(metrics)
	defer hub.Close()

	wsHandler := handler.NewWSHandler(hub, cfg.JWTSecret, cfg.AllowDevUserID, cfg.AllowedOrigins, stdLogger)
	mux := http.NewServeMux()
	mux.Handle("/ws", wsHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.WSPort,
		Handler:           metrics.HTTPMiddleware(otelhttp.NewHandler(mux, "notification-websocket")),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	adminServer := &http.Server{
		Addr:              ":" + cfg.AdminPort,
		Handler:           healthTracker.AdminMux(metrics),
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErrors := make(chan error, 3)
	go func() {
		logger.Info("grpc server listening", "addr", listener.Addr().String())
		if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			serveErrors <- fmt.Errorf("serve grpc: %w", err)
		}
	}()
	go func() {
		logger.Info("websocket gateway listening", "addr", httpServer.Addr, "path", "/ws")
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- fmt.Errorf("serve websocket gateway: %w", err)
		}
	}()
	go func() {
		logger.Info("admin server listening", "addr", adminServer.Addr,
			"endpoints", "/metrics,/healthz,/readyz")
		if err := adminServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErrors <- fmt.Errorf("serve admin: %w", err)
		}
	}()

	logger.Info("notification-service running",
		"grpc_port", cfg.Port,
		"ws_port", cfg.WSPort,
		"push_provider", pushProviderName,
		"email_provider", emailProviderName,
		"aggregation_window", cfg.AggregationWindow,
		"retention_days", cfg.RetentionDays)

	select {
	case err := <-serveErrors:
		return err
	case <-ctx.Done():
		logger.Info("shutdown signal received; draining connections")
	}

	healthTracker.SetReady(false)
	healthServer.SetServingStatus(grpcServiceName, healthpb.HealthCheckResponse_NOT_SERVING)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	// net/http does not close hijacked WebSocket connections, so close them
	// explicitly before asking the HTTP server to drain.
	hub.Close()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("websocket gateway shutdown failed", "error", err)
	}

	stopped := make(chan struct{})
	go func() {
		grpcServer.GracefulStop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-shutdownCtx.Done():
		logger.Warn("graceful stop timed out; forcing shutdown")
		grpcServer.Stop()
	}

	if err := adminServer.Shutdown(shutdownCtx); err != nil {
		logger.Warn("admin server shutdown failed", "error", err)
	}

	pushDispatcher.Wait()
	emailDispatcher.Wait()

	logger.Info("notification-service stopped")
	return nil
}

// buildPushProvider selects the configured push transport. A misconfigured
// external provider falls back to the log provider so a bad credentials file
// cannot take the whole service down.
func buildPushProvider(cfg *config.Config, logger *log.Logger) (dispatchers.PushProvider, string) {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	switch cfg.PushProvider {
	case "fcm":
		provider, err := dispatchers.NewFCMProvider(cfg.FCMCredentialsFile, cfg.FCMProjectID, httpClient, logger)
		if err != nil {
			logger.Printf("warning: FCM provider unavailable (%v); falling back to log provider", err)
			return dispatchers.NewLogPushProvider(logger), "log (fcm fallback)"
		}
		return provider, "fcm"
	case "apns":
		provider, err := dispatchers.NewAPNsProvider(cfg.APNSAuthKeyFile, cfg.APNSKeyID, cfg.APNSTeamID, cfg.APNSTopic, cfg.APNSSandbox, httpClient)
		if err != nil {
			logger.Printf("warning: APNs provider unavailable (%v); falling back to log provider", err)
			return dispatchers.NewLogPushProvider(logger), "log (apns fallback)"
		}
		return provider, "apns"
	default:
		return dispatchers.NewLogPushProvider(logger), "log"
	}
}

// buildEmailProvider selects the configured email transport.
func buildEmailProvider(cfg *config.Config, logger *log.Logger) (dispatchers.EmailProvider, string) {
	switch cfg.EmailProvider {
	case "smtp":
		provider, err := dispatchers.NewSMTPEmailProvider(cfg.SMTPHost, cfg.SMTPPort, cfg.SMTPUsername, cfg.SMTPPassword, cfg.EmailFrom)
		if err != nil {
			logger.Printf("warning: SMTP provider unavailable (%v); falling back to log provider", err)
			return dispatchers.NewLogEmailProvider(logger), "log (smtp fallback)"
		}
		return provider, "smtp"
	default:
		return dispatchers.NewLogEmailProvider(logger), "log"
	}
}

// maintainPartitions creates the rolling monthly partitions and starts the
// once-per-day retention sweep. Failure is logged rather than fatal so a
// transient database hiccup cannot block boot; the next tick retries.
func maintainPartitions(ctx context.Context, logger *log.Logger, repo repository.NotificationRepository, retentionDays int) {
	runSweep := func() {
		sweepCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := repo.EnsurePartitions(sweepCtx, 2); err != nil {
			logger.Printf("failed to ensure notification partitions: %v", err)
		}

		if retentionDays <= 0 {
			return
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)

		dropped, err := repo.DropPartitionsBefore(sweepCtx, cutoff)
		if err != nil {
			logger.Printf("failed to drop expired notification partitions: %v", err)
		} else if dropped > 0 {
			logger.Printf("dropped %d expired notification partitions", dropped)
		}

		pruned, err := repo.PruneBefore(sweepCtx, cutoff)
		if err != nil {
			logger.Printf("failed to prune expired notifications: %v", err)
		} else if pruned > 0 {
			logger.Printf("pruned %d notifications older than %d days", pruned, retentionDays)
		}
	}

	runSweep()

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runSweep()
			}
		}
	}()
}

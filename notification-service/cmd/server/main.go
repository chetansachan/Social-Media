package main

import (
	"context"
	"errors"
	"log"
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
	socialflowpb "github.com/chetansachan/socialflow/proto"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	cfg := config.Load()
	logger := log.New(os.Stdout, "notification-service ", log.LstdFlags|log.LUTC)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

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
		logger.Fatalf("failed to connect to PostgreSQL: %v", err)
	}
	defer dbPool.Close()
	logger.Printf("connected to PostgreSQL")

	// Redis powers unread badges, dedup locks, aggregation windows, push rate
	// limits, and WebSocket fan-out. Every one of those degrades gracefully, so
	// the service still boots without Redis.
	var redisClient *redis.Client
	client, err := database.ConnectRedis(ctx, cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Printf("warning: Redis unavailable at %s (%v); running in degraded mode", cfg.RedisAddr, err)
	} else {
		redisClient = client
		defer redisClient.Close()
		logger.Printf("connected to Redis at %s (db %d)", cfg.RedisAddr, cfg.RedisDB)
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
	userConn, err := grpc.NewClient(cfg.UserServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		logger.Printf("warning: failed to create user-service client: %v", err)
	} else {
		defer userConn.Close()
		actorDirectory = repository.NewActorDirectory(socialflowpb.NewUserServiceClient(userConn))
	}

	// -------------------------------------------------------------------------
	// Dispatchers
	// -------------------------------------------------------------------------
	realtime := dispatchers.NewRealtimeDispatcher(redisClient, logger)

	pushProvider, pushProviderName := buildPushProvider(cfg, logger)
	pushDispatcher := dispatchers.NewPushDispatcher(deviceRepo, pushProvider, cfg.WorkerCount, cfg.QueueSize, logger)
	pushDispatcher.Start(ctx)

	emailProvider, emailProviderName := buildEmailProvider(cfg, logger)
	emailDispatcher := dispatchers.NewEmailDispatcher(emailProvider, 2, cfg.QueueSize/4, logger)
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
		Logger:        logger,
	})

	// -------------------------------------------------------------------------
	// Event ingestion
	// -------------------------------------------------------------------------
	if redisClient != nil && cfg.EventConsumerEnabled {
		eventHandler := events.NewEventHandler(notificationService, logger)
		consumer := events.NewConsumer(redisClient, eventHandler, events.Options{
			Streams:       cfg.EventStreams,
			Group:         cfg.EventGroup,
			Consumer:      cfg.EventConsumerName,
			DLQStream:     cfg.EventDLQStream,
			ClaimInterval: cfg.EventClaimInterval,
			MaxAttempts:   cfg.EventMaxAttempts,
			Logger:        logger,
		})
		go func() {
			if err := consumer.Run(ctx); err != nil && ctx.Err() == nil {
				logger.Printf("event consumer stopped: %v", err)
			}
		}()
		logger.Printf("consuming %v in group %s as %s", cfg.EventStreams, cfg.EventGroup, cfg.EventConsumerName)
	} else if cfg.EventConsumerEnabled {
		logger.Printf("event consumer disabled: Redis is unavailable")
	}

	// -------------------------------------------------------------------------
	// Partition maintenance and retention
	// -------------------------------------------------------------------------
	maintainPartitions(ctx, logger, notificationRepo, cfg.RetentionDays)

	// -------------------------------------------------------------------------
	// gRPC API
	// -------------------------------------------------------------------------
	grpcServer := grpc.NewServer()
	socialflowpb.RegisterNotificationServiceServer(grpcServer, handler.NewGRPCHandler(notificationService, logger))

	healthServer := health.NewServer()
	healthServer.SetServingStatus("notification.NotificationService", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(grpcServer, healthServer)

	listener, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		logger.Fatalf("failed to listen on gRPC port %s: %v", cfg.Port, err)
	}

	// -------------------------------------------------------------------------
	// WebSocket gateway
	// -------------------------------------------------------------------------
	hub := handler.NewHub(ctx, redisClient, logger)
	defer hub.Close()

	wsHandler := handler.NewWSHandler(hub, cfg.JWTSecret, cfg.AllowDevUserID, cfg.AllowedOrigins, logger)
	mux := http.NewServeMux()
	mux.Handle("/ws", wsHandler)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	httpServer := &http.Server{
		Addr:              ":" + cfg.WSPort,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Printf("WebSocket gateway listening on :%s (path /ws)", cfg.WSPort)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Printf("WebSocket gateway stopped: %v", err)
			stop()
		}
	}()

	go func() {
		<-ctx.Done()
		logger.Printf("shutting down gracefully...")
		healthServer.SetServingStatus("notification.NotificationService", healthpb.HealthCheckResponse_NOT_SERVING)
		grpcServer.GracefulStop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Printf("Notification Service running")
	logger.Printf("  gRPC API          : :%s", cfg.Port)
	logger.Printf("  WebSocket gateway : :%s/ws", cfg.WSPort)
	logger.Printf("  Push provider     : %s", pushProviderName)
	logger.Printf("  Email provider    : %s", emailProviderName)
	logger.Printf("  Aggregation window: %s", cfg.AggregationWindow)
	logger.Printf("  Retention         : %d days", cfg.RetentionDays)

	if err := grpcServer.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
		logger.Fatalf("gRPC server failed: %v", err)
	}

	stop()
	pushDispatcher.Wait()
	emailDispatcher.Wait()
	logger.Printf("Notification Service stopped")
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

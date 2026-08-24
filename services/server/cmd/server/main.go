package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/httpapi"
	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"github.com/royal-flush/royal-flush/services/server/internal/infra"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
	"github.com/royal-flush/royal-flush/services/server/internal/voice"
	"github.com/royal-flush/royal-flush/services/server/migrations"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	port := env("PORT", "8080")
	development := env("ENVIRONMENT", "development") == "development"
	databaseURL := os.Getenv("DATABASE_URL")
	var database *infra.Postgres
	if databaseURL != "" {
		startupContext, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		var err error
		database, err = infra.OpenPostgres(startupContext, databaseURL)
		if err == nil {
			err = migrations.Run(startupContext, database.Pool())
		}
		cancel()
		if err != nil {
			logger.Error("postgres initialization failed", "error", err)
			os.Exit(1)
		}
		defer database.Close()
	} else if !development {
		logger.Error("DATABASE_URL is required outside development")
		os.Exit(1)
	} else {
		logger.Warn("using in-memory persistence because DATABASE_URL is not configured")
	}
	redisURL := os.Getenv("REDIS_URL")
	var roomLease room.Lease
	var redisClient interface{ Close() error }
	if redisURL != "" {
		client, err := infra.OpenRedis(redisURL)
		if err == nil {
			startupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = client.Ping(startupContext).Err()
			cancel()
		}
		if err != nil {
			logger.Error("redis initialization failed", "error", err)
			os.Exit(1)
		}
		redisClient = client
		roomLease = infra.NewRoomLease(client, "")
		defer redisClient.Close()
	} else if !development {
		logger.Error("REDIS_URL is required outside development")
		os.Exit(1)
	} else {
		logger.Warn("room leases are disabled because REDIS_URL is not configured")
	}
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		var err error
		instanceID, err = idgen.ID("instance")
		if err != nil {
			logger.Error("instance id generation failed", "error", err)
			os.Exit(1)
		}
	}
	applicationConfig := httpapi.Config{
		Development:    development,
		AllowedOrigins: splitCSV(os.Getenv("ALLOWED_ORIGINS")),
		Voice:          voice.Config{URL: os.Getenv("LIVEKIT_URL"), APIKey: os.Getenv("LIVEKIT_API_KEY"), APISecret: os.Getenv("LIVEKIT_API_SECRET")},
		RoomLease:      roomLease,
		InstanceID:     instanceID,
	}
	if database != nil {
		applicationConfig.ScoreStore = database
		applicationConfig.RoomStore = database
	}
	application := httpapi.New(applicationConfig, logger)
	defer application.Close()
	httpServer := &http.Server{
		Addr: ":" + port, Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second,
	}
	go func() {
		logger.Info("royal flush server listening", "address", httpServer.Addr, "development", development)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()
	shutdown, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-shutdown.Done()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		logger.Error("graceful shutdown failed", "error", err)
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func splitCSV(value string) []string {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

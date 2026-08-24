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
	"github.com/royal-flush/royal-flush/services/server/internal/infra"
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
	applicationConfig := httpapi.Config{
		Development:    development,
		AllowedOrigins: splitCSV(os.Getenv("ALLOWED_ORIGINS")),
		Voice:          voice.Config{URL: os.Getenv("LIVEKIT_URL"), APIKey: os.Getenv("LIVEKIT_API_KEY"), APISecret: os.Getenv("LIVEKIT_API_SECRET")},
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

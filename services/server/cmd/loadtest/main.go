package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/loadtest"
)

func main() {
	config := loadtest.Config{}
	output := "-"
	flag.StringVar(&config.Target, "target", "http://127.0.0.1:8080", "Royal Flush API base URL")
	flag.IntVar(&config.Connections, "connections", 1000, "WebSocket connections to keep open")
	flag.IntVar(&config.Rooms, "rooms", 120, "active rooms to create")
	flag.IntVar(&config.Actions, "actions", 120, "rooms in which to measure one real poker action")
	flag.IntVar(&config.Reconnects, "reconnects", 120, "rooms in which to measure snapshot recovery")
	flag.IntVar(&config.SetupConcurrency, "setup-concurrency", 16, "concurrent room and measurement workers")
	flag.IntVar(&config.SocketConcurrency, "socket-concurrency", 100, "concurrent WebSocket dial workers")
	flag.DurationVar(&config.RequestTimeout, "request-timeout", 10*time.Second, "timeout for each HTTP, WebSocket, or command operation")
	flag.DurationVar(&config.ActionP95Limit, "action-p95", 200*time.Millisecond, "maximum accepted action confirmation p95")
	flag.DurationVar(&config.ReconnectMaxLimit, "reconnect-max", 3*time.Second, "maximum accepted reconnect snapshot latency")
	flag.StringVar(&output, "output", "-", "JSON report path, or - for stdout")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	report, runErr := loadtest.Run(ctx, config)
	writeErr := writeReport(output, report)
	if runErr != nil {
		fmt.Fprintln(os.Stderr, "capacity test failed:", runErr)
	}
	if writeErr != nil {
		fmt.Fprintln(os.Stderr, "write report:", writeErr)
	}
	if runErr != nil || writeErr != nil || !report.Passed {
		os.Exit(1)
	}
}

func writeReport(path string, report loadtest.Report) error {
	destination := os.Stdout
	if path != "-" {
		file, err := os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		destination = file
	}
	encoder := json.NewEncoder(destination)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}

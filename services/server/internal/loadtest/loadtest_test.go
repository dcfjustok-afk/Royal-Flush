package loadtest

import (
	"testing"
	"time"
)

func TestConfigValidation(t *testing.T) {
	valid := Config{
		Target: "http://127.0.0.1:8080", Connections: 16, Rooms: 4, Actions: 4, Reconnects: 4,
		SetupConcurrency: 2, SocketConcurrency: 8, RequestTimeout: time.Second,
		ActionP95Limit: 200 * time.Millisecond, ReconnectMaxLimit: 3 * time.Second,
	}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{name: "relative target", mutate: func(config *Config) { config.Target = "/api" }},
		{name: "one player room", mutate: func(config *Config) { config.Connections = 4 }},
		{name: "too many action rooms", mutate: func(config *Config) { config.Actions = 5 }},
		{name: "too many reconnect rooms", mutate: func(config *Config) { config.Reconnects = 5 }},
		{name: "zero concurrency", mutate: func(config *Config) { config.SocketConcurrency = 0 }},
		{name: "zero threshold", mutate: func(config *Config) { config.ActionP95Limit = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			if err := config.Validate(); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestPercentileUsesNearestRankWithoutMutatingInput(t *testing.T) {
	values := []time.Duration{50 * time.Millisecond, 10 * time.Millisecond, 40 * time.Millisecond, 20 * time.Millisecond, 30 * time.Millisecond}
	if got := Percentile(values, 0.50); got != 30*time.Millisecond {
		t.Fatalf("p50 = %s", got)
	}
	if got := Percentile(values, 0.95); got != 50*time.Millisecond {
		t.Fatalf("p95 = %s", got)
	}
	if values[0] != 50*time.Millisecond {
		t.Fatal("percentile mutated the source slice")
	}
}

func TestLatencySummaryUsesRequestedEvaluation(t *testing.T) {
	values := make([]time.Duration, 20)
	for index := range values {
		values[index] = 100 * time.Millisecond
	}
	values[len(values)-1] = 250 * time.Millisecond
	if result := summarize(values, 200*time.Millisecond, "p95"); !result.Passed {
		t.Fatalf("p95 evaluation should pass: %#v", result)
	}
	if result := summarize(values, 200*time.Millisecond, "maximum"); result.Passed {
		t.Fatalf("maximum evaluation should fail: %#v", result)
	}
}

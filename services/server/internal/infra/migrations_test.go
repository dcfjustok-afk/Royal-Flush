package infra

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInitialMigrationContainsCriticalIntegrityConstraints(t *testing.T) {
	_, source, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(source), "..", "..", "migrations", "001_initial.sql")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sql := string(raw)
	required := []string{
		"seat_sessions_one_active_room_per_user",
		"score_ledger_idempotent_request",
		"score_addition_rate_limit",
		"seat_session_id text PRIMARY KEY",
		"PRIMARY KEY (room_id, version)",
		"request_id text NOT NULL UNIQUE",
	}
	for _, fragment := range required {
		if !strings.Contains(sql, fragment) {
			t.Errorf("migration is missing %q", fragment)
		}
	}
	settlementPath := filepath.Join(filepath.Dir(source), "..", "..", "migrations", "003_settlement_idempotency.sql")
	settlement, err := os.ReadFile(settlementPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settlement), "score_ledger_unique_settlement") {
		t.Fatal("settlement idempotency migration is missing its unique index")
	}
	eventPath := filepath.Join(filepath.Dir(source), "..", "..", "migrations", "004_room_event_sequence.sql")
	eventMigration, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(eventMigration), "room_events_room_version") {
		t.Fatal("room event sequence migration is missing its version index")
	}
}

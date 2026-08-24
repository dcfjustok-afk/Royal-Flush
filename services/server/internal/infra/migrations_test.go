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
}

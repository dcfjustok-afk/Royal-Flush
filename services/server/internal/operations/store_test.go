package operations

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestMemoryStoreModerationReportAndAuditLifecycle(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	store := NewMemoryStore()
	for _, identity := range []UserIdentity{
		{ID: "admin", Nickname: "运营", CreatedAt: now},
		{ID: "u1", Phone: "13800138000", Nickname: "玩家", CreatedAt: now},
	} {
		if err := store.UpsertUser(ctx, identity); err != nil {
			t.Fatal(err)
		}
	}
	user, duplicate, err := store.SetUserBanned(ctx, "admin", "u1", true, "违规行为", "ban-1", now)
	if err != nil || duplicate || !user.Banned {
		t.Fatalf("ban failed: %#v duplicate=%v err=%v", user, duplicate, err)
	}
	_, duplicate, err = store.SetUserBanned(ctx, "admin", "u1", false, "changed retry", "ban-1", now)
	if err != nil || !duplicate {
		t.Fatalf("ban retry was not idempotent: duplicate=%v err=%v", duplicate, err)
	}
	report, duplicate, err := store.CreateReport(ctx, ReportInput{
		ReporterID: "u1", Category: "conduct", Detail: "持续拖延操作", RequestID: "report-1", CreatedAt: now,
	})
	if err != nil || duplicate || report.Status != "open" {
		t.Fatalf("report creation failed: %#v duplicate=%v err=%v", report, duplicate, err)
	}
	resolved, duplicate, err := store.HandleReport(ctx, "admin", report.ID, "resolved", "已核查", "resolve-1", now.Add(time.Minute))
	if err != nil || duplicate || resolved.Status != "resolved" {
		t.Fatalf("report handling failed: %#v duplicate=%v err=%v", resolved, duplicate, err)
	}
	audits, err := store.ListAudits(ctx, 10)
	if err != nil || len(audits) != 2 {
		t.Fatalf("audit list = %#v, %v", audits, err)
	}
	if _, _, err := store.SetUserBanned(ctx, "admin", "missing", true, "违规", "ban-missing", now); !errors.Is(err, ErrUserNotFound) {
		t.Fatalf("missing user error = %v", err)
	}
}

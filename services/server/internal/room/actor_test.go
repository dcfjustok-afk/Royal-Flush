package room

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/chips"
	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

func TestActorSerializesCommandsAndKeepsConfigImmutable(t *testing.T) {
	ctx := context.Background()
	scores := score.NewService(nil)
	config := testConfig()
	manager := NewManager(scores)
	actor, err := manager.Create(ctx, config, Identity{ID: "u1", Name: "小北"})
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	config.ChipDenominations = append(config.ChipDenominations, 500)
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "阿岚"}, 4); err != nil {
		t.Fatal(err)
	}
	ready := json.RawMessage(`{"ready":true}`)
	for _, userID := range []string{"u1", "u2"} {
		if _, _, err := actor.Handle(ctx, userID, ClientCommand{Type: "room.ready", RequestID: "ready-" + userID, Payload: ready}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, _ := actor.Snapshot(ctx, "u1")
	startVersion := snapshot.Version
	if _, _, err := actor.Handle(ctx, "u1", ClientCommand{Type: "game.start", RequestID: "start-1"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = actor.Snapshot(ctx, "u1")
	if snapshot.Version <= startVersion || len(snapshot.AllowedChipDenominations) != 5 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	if snapshot.Street != "preflop" || len(snapshot.HoleCards) != 2 {
		t.Fatal("hand did not start with private cards")
	}
	other, _ := actor.Snapshot(ctx, "u2")
	if len(other.HoleCards) != 2 || (other.HoleCards[0] == snapshot.HoleCards[0] && other.HoleCards[1] == snapshot.HoleCards[1]) {
		t.Fatal("snapshots must contain only each local player's private cards")
	}
}

func TestRaiseValidationVersionAndIdempotency(t *testing.T) {
	ctx := context.Background()
	scores := score.NewService(nil)
	actor, _ := NewActor(testConfig(), Identity{ID: "u1", Name: "小北"}, scores, nil)
	defer actor.Close()
	_, _ = actor.Join(ctx, Identity{ID: "u2", Name: "阿岚"}, 1)
	ready := json.RawMessage(`{"ready":true}`)
	_, _, _ = actor.Handle(ctx, "u1", ClientCommand{Type: "room.ready", RequestID: "r1", Payload: ready})
	_, _, _ = actor.Handle(ctx, "u2", ClientCommand{Type: "room.ready", RequestID: "r2", Payload: ready})
	_, _, _ = actor.Handle(ctx, "u1", ClientCommand{Type: "game.start", RequestID: "start"})
	snapshot, _ := actor.Snapshot(ctx, "u1")
	actorID := "u1"
	if !localPlayer(snapshot).IsCurrentActor {
		actorID = "u2"
		snapshot, _ = actor.Snapshot(ctx, "u2")
	}
	illegal, _ := json.Marshal(map[string]any{"chips": []int64{200}})
	_, _, err := actor.Handle(ctx, actorID, ClientCommand{Type: "action.raise", RequestID: "bad-chip", ExpectedVersion: snapshot.Version, Payload: illegal})
	if !errors.Is(err, chips.ErrInvalidChip) {
		t.Fatalf("expected denomination rejection, got %v", err)
	}
	valid, _ := json.Marshal(map[string]any{"chips": []int64{10}})
	event, duplicate, err := actor.Handle(ctx, actorID, ClientCommand{Type: "action.raise", RequestID: "raise-1", ExpectedVersion: snapshot.Version, Payload: valid})
	if err != nil || duplicate {
		t.Fatalf("raise failed: %#v %v %v", event, duplicate, err)
	}
	retry, duplicate, err := actor.Handle(ctx, actorID, ClientCommand{Type: "action.raise", RequestID: "raise-1", ExpectedVersion: snapshot.Version, Payload: valid})
	if err != nil || !duplicate || retry.Version != event.Version {
		t.Fatalf("idempotent command failed: %#v %v %v", retry, duplicate, err)
	}
	_, _, err = actor.Handle(ctx, actorID, ClientCommand{Type: "action.fold", RequestID: "stale", ExpectedVersion: snapshot.Version})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
}

func TestScoreBroadcastIsPersistent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	scores := score.NewService(nil)
	actor, _ := NewActor(testConfig(), Identity{ID: "u1", Name: "小北"}, scores, nil)
	defer actor.Close()
	events, unsubscribe, err := actor.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	if err := actor.BroadcastScoreAddition(ctx, "u1", "add-1", 250, 1250); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-events:
		if event.Type != "score.self_added" {
			t.Fatalf("unexpected event: %#v", event)
		}
	case <-ctx.Done():
		t.Fatal("broadcast not received")
	}
	snapshot, _ := actor.Snapshot(context.Background(), "u1")
	if len(snapshot.Messages) < 2 || snapshot.Messages[0].Type != "score" {
		t.Fatalf("broadcast was not persisted: %#v", snapshot.Messages)
	}
}

func localPlayer(snapshot TableSnapshot) PlayerSnapshot {
	for _, player := range snapshot.Players {
		if player.IsLocal {
			return player
		}
	}
	return PlayerSnapshot{}
}

func testConfig() Config {
	return Config{Name: "周五夜场", MaxPlayers: 8, BlindPreset: "5/10", ActionSeconds: 20, VoiceEnabled: true, ChipDenominations: []int64{5, 10, 20, 50, 100}}
}

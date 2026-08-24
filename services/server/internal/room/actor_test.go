package room

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/chips"
	"github.com/royal-flush/royal-flush/services/server/internal/poker"
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

func TestHandStartedEventNeverBroadcastsPrivateCards(t *testing.T) {
	ctx := context.Background()
	scores := score.NewService(nil)
	actor, err := NewActor(testConfig(), Identity{ID: "u1", Name: "小北"}, scores, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if _, err := actor.Join(ctx, Identity{ID: "u2", Name: "阿岚"}, 1); err != nil {
		t.Fatal(err)
	}
	ready := json.RawMessage(`{"ready":true}`)
	for _, userID := range []string{"u1", "u2"} {
		if _, _, err := actor.Handle(ctx, userID, ClientCommand{Type: "room.ready", RequestID: "ready-" + userID, Payload: ready}); err != nil {
			t.Fatal(err)
		}
	}
	event, _, err := actor.Handle(ctx, "u1", ClientCommand{Type: "game.start", RequestID: "start-private-check"})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "holeCards") || strings.Contains(string(payload), "snapshot") {
		t.Fatalf("shared event exposed a personalized snapshot: %s", payload)
	}
}

func TestGameStartRequiresTwoConnectedReadyPlayers(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if _, err := actor.Join(ctx, Identity{ID: "offline", Name: "断线玩家"}, 1); err != nil {
		t.Fatal(err)
	}
	ready := json.RawMessage(`{"ready":true}`)
	for _, userID := range []string{"owner", "offline"} {
		if _, _, err := actor.Handle(ctx, userID, ClientCommand{Type: "room.ready", RequestID: "ready-" + userID, Payload: ready}); err != nil {
			t.Fatal(err)
		}
	}
	if err := actor.PlayerConnected(ctx, "offline"); err != nil {
		t.Fatal(err)
	}
	if err := actor.PlayerDisconnected(ctx, "offline"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "game.start", RequestID: "start"}); !errors.Is(err, ErrPlayersNotReady) {
		t.Fatalf("expected disconnected ready player to be excluded, got %v", err)
	}
}

func TestRoomOwnerControlsAndAutomaticTransferUseJoinOrder(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(score.NewService(nil))
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "early", Name: "先到玩家"}, 6); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "late", Name: "后到玩家"}, 1); err != nil {
		t.Fatal(err)
	}

	removeLate := json.RawMessage(`{"userId":"late"}`)
	if _, _, err := actor.Handle(ctx, "early", ClientCommand{Type: "room.remove_player", RequestID: "remove-forbidden", Payload: removeLate}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-owner removal should be forbidden, got %v", err)
	}
	removeOwner := json.RawMessage(`{"userId":"owner"}`)
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.remove_player", RequestID: "remove-owner", Payload: removeOwner}); !errors.Is(err, ErrCannotRemoveOwner) {
		t.Fatalf("owner should not remove themselves, got %v", err)
	}
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.remove_player", RequestID: "remove-late", Payload: removeLate}); err != nil {
		t.Fatal(err)
	}
	if manager.ActiveRoom("late") != "" {
		t.Fatal("removed player still owns an active seat")
	}
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.leave", RequestID: "owner-leaves"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := actor.Snapshot(ctx, "early")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.OwnerID != "early" {
		t.Fatalf("owner transferred by seat order instead of join order: %s", snapshot.OwnerID)
	}
}

func TestRotatingInviteCodeInvalidatesThePreviousCode(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(score.NewService(nil))
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	oldCode := actor.Code
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.rotate_invite", RequestID: "rotate-code"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Room(oldCode); ok {
		t.Fatal("previous invite code still resolves after rotation")
	}
	if resolved, ok := manager.Room(actor.Code); !ok || resolved != actor {
		t.Fatal("new invite code does not resolve to the room")
	}
}

func TestDisconnectedSeatIsRetainedAndMultipleConnectionsAreCounted(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "u1", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	actor.disconnectWait = 20 * time.Millisecond
	if _, err := actor.Join(ctx, Identity{ID: "u2", Name: "阿岚"}, 2); err != nil {
		t.Fatal(err)
	}
	if err := actor.PlayerConnected(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	if err := actor.PlayerConnected(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	if err := actor.PlayerDisconnected(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u2") == "disconnected" {
		t.Fatal("closing one of multiple connections marked the player disconnected")
	}
	if err := actor.PlayerDisconnected(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u2") != "disconnected" {
		t.Fatal("last connection close did not mark the player disconnected")
	}
	if err := actor.PlayerConnected(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	snapshot, _ = actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u2") == "" {
		t.Fatal("reconnected player lost the retained seat")
	}
	if err := actor.PlayerDisconnected(ctx, "u2"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot, _ = actor.Snapshot(ctx, "u1")
		if playerStatus(snapshot, "u2") == "" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("disconnected seat was not released after the retention window")
}

func TestEmptyRoomExpiresUnlessAnotherPlayerJoins(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(score.NewService(nil))
	manager.emptyWait = 20 * time.Millisecond
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.leave", RequestID: "leave-empty"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Join(ctx, actor.Code, Identity{ID: "u2", Name: "接替玩家"}, 3); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := manager.Room(actor.ID); !ok {
		t.Fatal("room expired after its empty timer was cancelled")
	}
	snapshot, err := actor.Snapshot(ctx, "u2")
	if err != nil || snapshot.OwnerID != "u2" {
		t.Fatalf("first player returning to an empty room did not become owner: %#v %v", snapshot, err)
	}
	if _, _, err := actor.Handle(ctx, "u2", ClientCommand{Type: "room.leave", RequestID: "leave-expire"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := manager.Room(actor.ID); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("empty room was not expired after the retention window")
}

func TestEmptyRoomJoinRollsBackWhenOwnerPersistenceFails(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.leave", RequestID: "leave-owner"}); err != nil {
		t.Fatal(err)
	}
	actor.onOwnerChanged = func(string) error { return errors.New("database unavailable") }
	if _, err := actor.Join(ctx, Identity{ID: "u2", Name: "接替玩家"}, 3); err == nil {
		t.Fatal("join succeeded after owner persistence failed")
	}
	public, err := actor.PublicSnapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(public.Players) != 0 || public.OwnerID != "" {
		t.Fatalf("failed join left room state behind: %#v", public)
	}
}

func TestManagerAcquiresRenewsAndReleasesRoomLease(t *testing.T) {
	ctx := context.Background()
	lease := &recordingLease{acquire: true}
	manager := NewManagerWithInfrastructure(score.NewService(nil), nil, lease, "instance-a")
	manager.emptyWait = 10 * time.Millisecond
	manager.leaseTTL = 30 * time.Millisecond
	manager.leaseRenew = 5 * time.Millisecond
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	time.Sleep(12 * time.Millisecond)
	if lease.renewCount() == 0 {
		t.Fatal("room lease was not renewed")
	}
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.leave", RequestID: "leave-leased-room"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if lease.releaseCount() > 0 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("room lease was not released after empty-room expiry")
}

func TestManagerRejectsUnavailableLeaseAndClosesOnOwnershipLoss(t *testing.T) {
	ctx := context.Background()
	unavailable := &recordingLease{}
	manager := NewManagerWithInfrastructure(score.NewService(nil), nil, unavailable, "instance-a")
	if _, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"}); !errors.Is(err, ErrRoomLeaseUnavailable) {
		t.Fatalf("expected unavailable lease error, got %v", err)
	}

	lost := &recordingLease{acquire: true, renewErr: ErrLeaseNotOwned}
	manager = NewManagerWithInfrastructure(score.NewService(nil), nil, lost, "instance-a")
	manager.leaseRenew = 5 * time.Millisecond
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := manager.Room(actor.ID); !ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("room remained active after lease ownership was lost")
}

func TestManagerPersistsOwnerSeatJoinedSeatsAndCommands(t *testing.T) {
	ctx := context.Background()
	store := &recordingRoomStore{}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if store.createdOwner.UserID != "owner" || store.createdOwner.Seat != 0 || store.createdOwner.AllocatedPoints != InitialTablePoints {
		t.Fatalf("owner seat was not persisted with the room: %#v", store.createdOwner)
	}
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "玩家二"}, 2); err != nil {
		t.Fatal(err)
	}
	if len(store.openedSeats) != 1 || store.openedSeats[0].UserID != "u2" {
		t.Fatalf("joined seat was not persisted: %#v", store.openedSeats)
	}
	if _, _, err := actor.Handle(ctx, "u2", ClientCommand{Type: "room.quick_message", RequestID: "persisted-command", Payload: json.RawMessage(`{"message":"好牌"}`)}); err != nil {
		t.Fatal(err)
	}
	if len(store.events) == 0 || store.events[len(store.events)-1].RequestID != "persisted-command" {
		t.Fatalf("command event was not persisted: %#v", store.events)
	}
}

func TestManagerRestoresAnActiveHandAndPrivateCards(t *testing.T) {
	ctx := context.Background()
	store := &statefulRoomStore{states: make(map[string]PersistentState)}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "玩家二"}, 2); err != nil {
		t.Fatal(err)
	}
	ready := json.RawMessage(`{"ready":true}`)
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.ready", RequestID: "ready-owner", Payload: ready}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := actor.Handle(ctx, "u2", ClientCommand{Type: "room.ready", RequestID: "ready-u2", Payload: ready}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "game.start", RequestID: "start-before-restart"}); err != nil {
		t.Fatal(err)
	}
	manager.Close()

	restoredManager := NewManagerWithStore(score.NewService(nil), store)
	t.Cleanup(restoredManager.Close)
	if err := restoredManager.Restore(ctx); err != nil {
		t.Fatal(err)
	}
	restoredActor, ok := restoredManager.Room(actor.ID)
	if !ok {
		t.Fatal("active room was not restored")
	}
	snapshot, err := restoredActor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Street != poker.StreetPreflop || len(snapshot.HoleCards) != 2 {
		t.Fatalf("active hand or private cards were not restored: %#v", snapshot)
	}
	if playerStatus(snapshot, "owner") != "disconnected" || playerStatus(snapshot, "u2") != "disconnected" {
		t.Fatalf("restored players did not enter disconnect retention: %#v", snapshot.Players)
	}
}

type recordingLease struct {
	mu       sync.Mutex
	acquire  bool
	renews   int
	releases int
	renewErr error
}

type recordingRoomStore struct {
	createdOwner roomSeatCopy
	openedSeats  []roomSeatCopy
	events       []Envelope
}

type statefulRoomStore struct {
	recordingRoomStore
	states map[string]PersistentState
}

func (s *statefulRoomStore) SaveRoomState(_ context.Context, state PersistentState) error {
	s.states[state.Room.ID] = state
	return nil
}

func (s *statefulRoomStore) LoadRoomStates(context.Context) ([]PersistentState, error) {
	states := make([]PersistentState, 0, len(s.states))
	for _, state := range s.states {
		if !state.Ended {
			states = append(states, state)
		}
	}
	return states, nil
}

type roomSeatCopy = SeatRecord

func (s *recordingRoomStore) CreateRoom(_ context.Context, _ Record, owner SeatRecord) error {
	s.createdOwner = owner
	return nil
}

func (s *recordingRoomStore) OpenSeat(_ context.Context, seat SeatRecord, _ bool) error {
	s.openedSeats = append(s.openedSeats, seat)
	return nil
}

func (s *recordingRoomStore) AddSeatAllocation(context.Context, string, int64) error {
	return nil
}

func (s *recordingRoomStore) UpdateRoomCode(context.Context, string, string, string) error {
	return nil
}

func (s *recordingRoomStore) UpdateRoomOwner(context.Context, string, string) error {
	return nil
}

func (s *recordingRoomStore) EndRoom(context.Context, string, time.Time) error {
	return nil
}

func (s *recordingRoomStore) AppendRoomEvent(_ context.Context, _ string, event Envelope) error {
	s.events = append(s.events, event)
	return nil
}

func (l *recordingLease) Acquire(context.Context, string, string, time.Duration) (bool, error) {
	return l.acquire, nil
}

func (l *recordingLease) Renew(context.Context, string, string, time.Duration) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.renews++
	return l.renewErr
}

func (l *recordingLease) Release(context.Context, string, string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.releases++
	return nil
}

func (l *recordingLease) renewCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.renews
}

func (l *recordingLease) releaseCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.releases
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

func playerStatus(snapshot TableSnapshot, userID string) string {
	for _, player := range snapshot.Players {
		if player.ID == userID {
			return player.Status
		}
	}
	return ""
}

func testConfig() Config {
	return Config{Name: "周五夜场", MaxPlayers: 8, BlindPreset: "5/10", ActionSeconds: 20, VoiceEnabled: true, ChipDenominations: []int64{5, 10, 20, 50, 100}}
}

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

func TestUpdatingIdentitySynchronizesRoomSnapshotAndEvent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "旧昵称"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	events, unsubscribe, err := actor.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	if err := actor.UpdateIdentity(ctx, Identity{ID: "owner", Name: "新昵称"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if localPlayer(snapshot).Name != "新昵称" {
		t.Fatalf("room snapshot kept stale nickname: %#v", snapshot.Players)
	}
	select {
	case event := <-events:
		if event.Type != "room.player_profile_updated" {
			t.Fatalf("unexpected profile event: %#v", event)
		}
	case <-ctx.Done():
		t.Fatal("profile update event was not published")
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

func TestInviteRotationPersistenceFailureKeepsOldCodeAndManagerMapping(t *testing.T) {
	ctx := context.Background()
	store := &statefulRoomStore{states: make(map[string]PersistentState)}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	oldCode := actor.Code
	store.saveErr = errors.New("database unavailable")
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.rotate_invite", RequestID: "rotate-fails"}); !errors.Is(err, store.saveErr) {
		t.Fatalf("rotation error = %v, want persistence failure", err)
	}
	if actor.Code != oldCode {
		t.Fatalf("failed rotation changed actor code from %q to %q", oldCode, actor.Code)
	}
	if resolved, ok := manager.Room(oldCode); !ok || resolved != actor {
		t.Fatal("failed rotation invalidated the working invite code")
	}
}

func TestOwnerTransferSideEffectRunsOnlyAfterStatePersistence(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if _, err := actor.Join(ctx, Identity{ID: "u2", Name: "玩家二"}, 1); err != nil {
		t.Fatal(err)
	}
	persistErr := errors.New("database unavailable")
	ownerSideEffects := 0
	actor.onOwnerChanged = func(string) error {
		ownerSideEffects++
		return nil
	}
	actor.onEvent = func(string, Envelope, PersistentState) error { return persistErr }
	payload := json.RawMessage(`{"userId":"u2"}`)
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.transfer_owner", RequestID: "transfer-fails", Payload: payload}); !errors.Is(err, persistErr) {
		t.Fatalf("transfer error = %v, want persistence failure", err)
	}
	snapshot, err := actor.Snapshot(ctx, "owner")
	if err != nil || snapshot.OwnerID != "owner" {
		t.Fatalf("failed transfer changed owner: snapshot=%#v err=%v", snapshot, err)
	}
	if ownerSideEffects != 0 {
		t.Fatalf("owner persistence ran %d times before room state committed", ownerSideEffects)
	}
	actor.onEvent = nil
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.transfer_owner", RequestID: "transfer-fails", Payload: payload}); err != nil {
		t.Fatal(err)
	}
	if ownerSideEffects != 1 {
		t.Fatalf("successful transfer side effect count = %d, want 1", ownerSideEffects)
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

func TestAtomicRoomCreationFailureLeavesNoOwnerSeatOrState(t *testing.T) {
	ctx := context.Background()
	persistErr := errors.New("database unavailable")
	store := &statefulRoomStore{states: make(map[string]PersistentState), saveErr: persistErr}
	manager := NewManagerWithStore(score.NewService(nil), store)
	t.Cleanup(manager.Close)
	if _, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"}); !errors.Is(err, persistErr) {
		t.Fatalf("create error = %v, want persistence failure", err)
	}
	if store.createdOwner.ID != "" || len(store.states) != 0 {
		t.Fatalf("failed creation left database artifacts: owner=%#v states=%#v", store.createdOwner, store.states)
	}
	if manager.ActiveRoom("owner") != "" {
		t.Fatal("failed creation retained active membership")
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

func TestManagerRestoresPlayerJoinedImmediatelyBeforeRestart(t *testing.T) {
	ctx := context.Background()
	store := &statefulRoomStore{states: make(map[string]PersistentState)}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "刚入座玩家"}, 3); err != nil {
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
		t.Fatal("room disappeared after restart immediately following a join")
	}
	snapshot, err := restoredActor.Snapshot(ctx, "u2")
	if err != nil {
		t.Fatalf("newly joined player disappeared after restart: %v", err)
	}
	if playerStatus(snapshot, "u2") == "" || len(snapshot.Players) != 2 {
		t.Fatalf("restored room lost the newly joined seat: %#v", snapshot.Players)
	}
}

func TestJoinPersistenceFailureRestoresActorState(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	before, err := actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	persistErr := errors.New("database unavailable")
	actor.onJoin = func(SeatRecord, bool, string, Envelope, PersistentState) error { return persistErr }
	if _, err := actor.Join(ctx, Identity{ID: "u2", Name: "玩家二"}, 2); !errors.Is(err, persistErr) {
		t.Fatalf("expected persistence error, got %v", err)
	}
	after, err := actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || len(after.Players) != 1 || playerStatus(after, "u2") != "" {
		t.Fatalf("failed join leaked into actor state: before=%#v after=%#v", before, after)
	}
}

type flakySettlementScores struct {
	failUser     string
	failuresLeft int
	applied      map[string]score.Result
}

func newFlakySettlementScores(failUser string, failures int) *flakySettlementScores {
	return &flakySettlementScores{failUser: failUser, failuresLeft: failures, applied: make(map[string]score.Result)}
}

func (s *flakySettlementScores) Balance(string) int64 { return 1000 }

func (s *flakySettlementScores) ApplySettlement(userID, _ string, seatSessionID string, _ int64) (score.Result, error) {
	if result, ok := s.applied[seatSessionID]; ok {
		return result, nil
	}
	if userID == s.failUser && s.failuresLeft > 0 {
		s.failuresLeft--
		return score.Result{}, errors.New("settlement storage unavailable")
	}
	result := score.Result{Balance: 1000}
	s.applied[seatSessionID] = result
	return result, nil
}

func (s *flakySettlementScores) applicationCount() int { return len(s.applied) }

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
	states      map[string]PersistentState
	allocations map[string]int64
	saveErr     error
}

func (s *statefulRoomStore) SaveRoomState(_ context.Context, state PersistentState) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.states[state.Room.ID] = state
	return nil
}

func (s *statefulRoomStore) CreateRoomAndSaveState(ctx context.Context, record Record, owner SeatRecord, state PersistentState) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	if err := s.CreateRoom(ctx, record, owner); err != nil {
		return err
	}
	return s.SaveRoomState(ctx, state)
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

func (s *statefulRoomStore) AppendRoomEventAndState(ctx context.Context, actorUserID string, event Envelope, state PersistentState) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	if event.Type == "room.table_points_refilled" {
		for _, player := range state.Game.Seats {
			if player != nil && player.UserID == actorUserID {
				if s.allocations == nil {
					s.allocations = make(map[string]int64)
				}
				s.allocations[player.SeatSessionID] = player.Allocated
				break
			}
		}
	}
	if err := s.AppendRoomEvent(ctx, actorUserID, event); err != nil {
		return err
	}
	return s.SaveRoomState(ctx, state)
}

func (s *statefulRoomStore) OpenSeatAndAppendRoomEventAndState(ctx context.Context, seat SeatRecord, claimOwnership bool, actorUserID string, event Envelope, state PersistentState) error {
	if err := s.OpenSeat(ctx, seat, claimOwnership); err != nil {
		return err
	}
	if err := s.AppendRoomEvent(ctx, actorUserID, event); err != nil {
		return err
	}
	return s.SaveRoomState(ctx, state)
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

func TestProcessedCommandKeysRemainValidPostgresJSON(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "user-one", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	payload := json.RawMessage(`{"ready":true}`)
	command := ClientCommand{Type: "room.ready", RequestID: "ready-request", Payload: payload}
	event, duplicate, err := actor.Handle(ctx, "user-one", command)
	if err != nil || duplicate {
		t.Fatalf("first command failed: event=%#v duplicate=%v err=%v", event, duplicate, err)
	}
	retry, duplicate, err := actor.Handle(ctx, "user-one", command)
	if err != nil || !duplicate || retry.Version != event.Version {
		t.Fatalf("encoded idempotency key lost duplicate detection: event=%#v duplicate=%v err=%v", retry, duplicate, err)
	}
	if err := actor.BroadcastScoreAddition(ctx, "user-one", "score-request", 10, 1010); err != nil {
		t.Fatal(err)
	}
	state, err := actor.PersistentState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `\u0000`) {
		t.Fatalf("persisted state still contains a PostgreSQL-incompatible Unicode escape: %s", raw)
	}
	if strings.ContainsRune(processedKey("command", "user\x00one", "request\x00one"), '\x00') {
		t.Fatal("encoded idempotency key retained a control character")
	}
}

func TestCommandRejectsControlCharactersInRequestID(t *testing.T) {
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	before, err := actor.Snapshot(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}

	payload, _ := json.Marshal(map[string]any{"ready": true})
	_, _, err = actor.Handle(context.Background(), "owner", ClientCommand{Type: "room.ready", RequestID: "ready\x00request", Payload: payload})
	if !errors.Is(err, score.ErrRequestID) {
		t.Fatalf("Handle error = %v, want score.ErrRequestID", err)
	}
	after, err := actor.Snapshot(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	if after.Version != before.Version || after.Players[0].IsReady != before.Players[0].IsReady {
		t.Fatalf("invalid request id changed room: before=%+v after=%+v", before, after)
	}
}

func TestCommandPersistenceFailureRestoresActorState(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "owner", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	before, err := actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	persistErr := errors.New("database unavailable")
	actor.onEvent = func(string, Envelope, PersistentState) error { return persistErr }
	payload := json.RawMessage(`{"ready":true}`)
	_, _, err = actor.Handle(ctx, "owner", ClientCommand{Type: "room.ready", RequestID: "ready-fails", Payload: payload})
	if !errors.Is(err, persistErr) {
		t.Fatalf("expected persistence error, got %v", err)
	}
	afterFailure, err := actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if afterFailure.Version != before.Version || localPlayer(afterFailure).IsReady {
		t.Fatalf("failed command leaked into actor state: before=%#v after=%#v", before, afterFailure)
	}
	state, err := actor.PersistentState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Processed) != 0 {
		t.Fatalf("failed command retained idempotency state: %#v", state.Processed)
	}
	actor.onEvent = nil
	if _, duplicate, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.ready", RequestID: "ready-fails", Payload: payload}); err != nil || duplicate {
		t.Fatalf("retry after rollback failed: duplicate=%v err=%v", duplicate, err)
	}
	afterRetry, err := actor.Snapshot(ctx, "owner")
	if err != nil || afterRetry.Version != before.Version+1 || !localPlayer(afterRetry).IsReady {
		t.Fatalf("retry did not apply exactly once: snapshot=%#v err=%v", afterRetry, err)
	}
}

func TestSettlementFailureKeepsSeatAndAllowsSameRequestRetry(t *testing.T) {
	ctx := context.Background()
	scores := newFlakySettlementScores("u2", 1)
	manager := NewManager(scores)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "玩家二"}, 2); err != nil {
		t.Fatal(err)
	}

	if _, _, err := actor.Handle(ctx, "u2", ClientCommand{Type: "room.leave", RequestID: "leave-retry"}); err == nil {
		t.Fatal("leave succeeded while settlement storage was unavailable")
	}
	snapshot, err := actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Players) != 2 || playerStatus(snapshot, "u2") != "active" || manager.ActiveRoom("u2") != actor.ID {
		t.Fatalf("failed settlement removed the active seat: players=%#v active=%q", snapshot.Players, manager.ActiveRoom("u2"))
	}

	if _, duplicate, err := actor.Handle(ctx, "u2", ClientCommand{Type: "room.leave", RequestID: "leave-retry"}); err != nil || duplicate {
		t.Fatalf("same request retry failed: duplicate=%v err=%v", duplicate, err)
	}
	snapshot, err = actor.Snapshot(ctx, "owner")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Players) != 1 || playerStatus(snapshot, "u2") != "" || manager.ActiveRoom("u2") != "" {
		t.Fatalf("successful settlement retained the seat: players=%#v active=%q", snapshot.Players, manager.ActiveRoom("u2"))
	}
	if scores.applicationCount() != 1 {
		t.Fatalf("settlement applied %d times, want exactly once", scores.applicationCount())
	}
}

func TestRefillPersistenceFailureDoesNotDoubleSeatAllocation(t *testing.T) {
	ctx := context.Background()
	store := &statefulRoomStore{states: make(map[string]PersistentState), allocations: make(map[string]int64)}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	var seatSessionID string
	if _, err := actor.call(ctx, func() (any, error) {
		player := actor.game.Seats[0]
		player.Stack = 0
		player.Away = true
		seatSessionID = player.SeatSessionID
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	store.allocations[seatSessionID] = InitialTablePoints
	store.saveErr = errors.New("database unavailable")
	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.refill", RequestID: "refill-retry"}); !errors.Is(err, store.saveErr) {
		t.Fatalf("refill error = %v, want persistence failure", err)
	}
	snapshot, _ := actor.Snapshot(ctx, "owner")
	if localPlayer(snapshot).TablePoints != 0 || store.allocations[seatSessionID] != InitialTablePoints {
		t.Fatalf("failed refill changed allocation: player=%#v stored=%d", localPlayer(snapshot), store.allocations[seatSessionID])
	}

	store.saveErr = nil
	if _, duplicate, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.refill", RequestID: "refill-retry"}); err != nil || duplicate {
		t.Fatalf("refill retry failed: duplicate=%v err=%v", duplicate, err)
	}
	snapshot, _ = actor.Snapshot(ctx, "owner")
	if localPlayer(snapshot).TablePoints != InitialTablePoints || store.allocations[seatSessionID] != 2*InitialTablePoints {
		t.Fatalf("refill retry was not applied exactly once: player=%#v stored=%d", localPlayer(snapshot), store.allocations[seatSessionID])
	}
}

func TestRoomEndRetriesPartialSettlementsExactlyOnce(t *testing.T) {
	ctx := context.Background()
	scores := newFlakySettlementScores("u2", 1)
	manager := NewManager(scores)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(manager.Close)
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "玩家二"}, 2); err != nil {
		t.Fatal(err)
	}

	if _, _, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.end", RequestID: "end-retry"}); err == nil {
		t.Fatal("room end succeeded after a partial settlement failure")
	}
	snapshot, err := actor.Snapshot(ctx, "owner")
	if err != nil || snapshot.Ended || len(snapshot.Players) != 2 {
		t.Fatalf("failed room end leaked actor changes: snapshot=%#v err=%v", snapshot, err)
	}
	if manager.ActiveRoom("owner") != actor.ID || manager.ActiveRoom("u2") != actor.ID {
		t.Fatal("partial settlement released active membership before room state committed")
	}

	if _, duplicate, err := actor.Handle(ctx, "owner", ClientCommand{Type: "room.end", RequestID: "end-retry"}); err != nil || duplicate {
		t.Fatalf("room end retry failed: duplicate=%v err=%v", duplicate, err)
	}
	public, err := actor.PublicSnapshot(ctx)
	if err != nil || !public.Ended || len(public.Players) != 0 {
		t.Fatalf("room end retry did not finish: snapshot=%#v err=%v", public, err)
	}
	if scores.applicationCount() != 2 {
		t.Fatalf("settlements applied %d times for two seats", scores.applicationCount())
	}
}

func TestSettledDepartureDoesNotReappearAfterRestart(t *testing.T) {
	ctx := context.Background()
	store := &statefulRoomStore{states: make(map[string]PersistentState)}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "owner", Name: "房主"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Join(ctx, actor.ID, Identity{ID: "u2", Name: "离桌玩家"}, 2); err != nil {
		t.Fatal(err)
	}
	if _, _, err := actor.Handle(ctx, "u2", ClientCommand{Type: "room.leave", RequestID: "leave-before-restart"}); err != nil {
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
		t.Fatal("active room disappeared after restart")
	}
	if restoredManager.ActiveRoom("u2") != "" {
		t.Fatal("departed player regained an active room after restart")
	}
	snapshot, err := restoredActor.Snapshot(ctx, "owner")
	if err != nil || len(snapshot.Players) != 1 || playerStatus(snapshot, "u2") != "" {
		t.Fatalf("departed seat reappeared after restart: players=%#v err=%v", snapshot.Players, err)
	}
}

func TestManagerSwitchesWaitingRoomsAndPreservesCurrentRoomOnTargetFailure(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(score.NewService(nil))
	first, err := manager.Create(ctx, testConfig(), Identity{ID: "switcher", Name: "换桌玩家"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := manager.Create(ctx, testConfig(), Identity{ID: "owner-b", Name: "房主 B"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := manager.Join(ctx, second.ID, Identity{ID: "occupied", Name: "占座玩家"}, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Join(ctx, second.ID, Identity{ID: "switcher", Name: "换桌玩家"}, 1); !errors.Is(err, poker.ErrSeatOccupied) {
		t.Fatalf("expected occupied target rejection, got %v", err)
	}
	if manager.ActiveRoom("switcher") != first.ID {
		t.Fatalf("failed target switch removed current membership: %q", manager.ActiveRoom("switcher"))
	}
	if _, err := first.Snapshot(ctx, "switcher"); err != nil {
		t.Fatalf("failed target switch removed player from current room: %v", err)
	}
	snapshot, err := manager.Join(ctx, second.ID, Identity{ID: "switcher", Name: "换桌玩家"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.RoomID != second.ID || manager.ActiveRoom("switcher") != second.ID {
		t.Fatalf("room switch did not select target: snapshot=%q active=%q", snapshot.RoomID, manager.ActiveRoom("switcher"))
	}
	if _, err := first.Snapshot(ctx, "switcher"); !errors.Is(err, ErrPlayerNotSeated) {
		t.Fatalf("room switch retained previous seat: %v", err)
	}
}

func TestManagerDoesNotSwitchRoomsDuringAnActiveHand(t *testing.T) {
	ctx := context.Background()
	manager := NewManager(score.NewService(nil))
	first, err := manager.Create(ctx, testConfig(), Identity{ID: "switcher", Name: "换桌玩家"})
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if _, err := manager.Join(ctx, first.ID, Identity{ID: "player-two", Name: "玩家二"}, 1); err != nil {
		t.Fatal(err)
	}
	ready := json.RawMessage(`{"ready":true}`)
	for _, userID := range []string{"switcher", "player-two"} {
		if _, _, err := first.Handle(ctx, userID, ClientCommand{Type: "room.ready", RequestID: "ready-" + userID, Payload: ready}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := first.Handle(ctx, "switcher", ClientCommand{Type: "game.start", RequestID: "start"}); err != nil {
		t.Fatal(err)
	}
	second, err := manager.Create(ctx, testConfig(), Identity{ID: "owner-b", Name: "房主 B"})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if _, err := manager.Join(ctx, second.ID, Identity{ID: "switcher", Name: "换桌玩家"}, 1); !errors.Is(err, ErrRoomSwitchInHand) {
		t.Fatalf("expected active-hand switch rejection, got %v", err)
	}
	if manager.ActiveRoom("switcher") != first.ID {
		t.Fatalf("active-hand rejection changed membership: %q", manager.ActiveRoom("switcher"))
	}
}

func TestRoomActivityDoesNotPostponeTheActionDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	actor, err := NewActor(testConfig(), Identity{ID: "u1", Name: "房主"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if _, err := actor.Join(ctx, Identity{ID: "u2", Name: "玩家二"}, 1); err != nil {
		t.Fatal(err)
	}
	ready := json.RawMessage(`{"ready":true}`)
	for _, userID := range []string{"u1", "u2"} {
		if _, _, err := actor.Handle(ctx, userID, ClientCommand{Type: "room.ready", RequestID: "ready-" + userID, Payload: ready}); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := actor.Handle(ctx, "u1", ClientCommand{Type: "game.start", RequestID: "start"}); err != nil {
		t.Fatal(err)
	}
	events, unsubscribe, err := actor.Subscribe(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	if _, err := actor.call(ctx, func() (any, error) {
		actor.scheduleActionTimeoutAt(time.Now().Add(40 * time.Millisecond))
		return nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := actor.Snapshot(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	nonActor := "u1"
	if localPlayer(snapshot).IsCurrentActor {
		nonActor = "u2"
	}
	if err := actor.PlayerConnected(ctx, nonActor); err != nil {
		t.Fatal(err)
	}
	if err := actor.PlayerDisconnected(ctx, nonActor); err != nil {
		t.Fatal(err)
	}
	message := json.RawMessage(`{"message":"好牌"}`)
	if _, _, err := actor.Handle(ctx, nonActor, ClientCommand{Type: "room.quick_message", RequestID: "message", Payload: message}); err != nil {
		t.Fatal(err)
	}
	for {
		select {
		case event := <-events:
			if event.Type == "game.action_timed_out" {
				return
			}
		case <-ctx.Done():
			t.Fatal("room activity postponed the action timeout")
		}
	}
}

func TestPresencePersistenceFailureRollsBackDisconnectAndReconnect(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "u1", Name: "小北"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	if err := actor.PlayerConnected(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	persistErr := errors.New("database unavailable")
	actor.onEvent = func(string, Envelope, PersistentState) error { return persistErr }
	if err := actor.PlayerDisconnected(ctx, "u1"); !errors.Is(err, persistErr) {
		t.Fatalf("disconnect error = %v, want persistence failure", err)
	}
	snapshot, _ := actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u1") != "active" {
		t.Fatalf("failed disconnect changed presence: %#v", snapshot.Players)
	}

	actor.onEvent = nil
	if err := actor.PlayerDisconnected(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u1") != "disconnected" {
		t.Fatalf("successful disconnect was not applied: %#v", snapshot.Players)
	}

	actor.onEvent = func(string, Envelope, PersistentState) error { return persistErr }
	if err := actor.PlayerConnected(ctx, "u1"); !errors.Is(err, persistErr) {
		t.Fatalf("reconnect error = %v, want persistence failure", err)
	}
	snapshot, _ = actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u1") != "disconnected" {
		t.Fatalf("failed reconnect changed presence: %#v", snapshot.Players)
	}
	actor.onEvent = nil
	if err := actor.PlayerConnected(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	snapshot, _ = actor.Snapshot(ctx, "u1")
	if playerStatus(snapshot, "u1") != "active" {
		t.Fatalf("reconnect retry did not restore presence: %#v", snapshot.Players)
	}
}

func TestScoreBroadcastPersistenceFailureRestoresMessagesAndIdempotency(t *testing.T) {
	ctx := context.Background()
	actor, err := NewActor(testConfig(), Identity{ID: "u1", Name: "小北"}, score.NewService(nil), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer actor.Close()
	before, _ := actor.Snapshot(ctx, "u1")
	persistErr := errors.New("database unavailable")
	actor.onEvent = func(string, Envelope, PersistentState) error { return persistErr }
	if err := actor.BroadcastScoreAddition(ctx, "u1", "add-retry", 250, 1250); !errors.Is(err, persistErr) {
		t.Fatalf("broadcast error = %v, want persistence failure", err)
	}
	afterFailure, _ := actor.Snapshot(ctx, "u1")
	if afterFailure.Version != before.Version || len(afterFailure.Messages) != len(before.Messages) {
		t.Fatalf("failed broadcast leaked state: before=%#v after=%#v", before.Messages, afterFailure.Messages)
	}
	actor.onEvent = nil
	if err := actor.BroadcastScoreAddition(ctx, "u1", "add-retry", 250, 1250); err != nil {
		t.Fatalf("broadcast retry failed: %v", err)
	}
	afterRetry, _ := actor.Snapshot(ctx, "u1")
	if afterRetry.Version != before.Version+1 || len(afterRetry.Messages) != len(before.Messages)+1 {
		t.Fatalf("broadcast retry did not apply once: %#v", afterRetry.Messages)
	}
}

func TestScoreAndResetBroadcastsSurviveRestart(t *testing.T) {
	ctx := context.Background()
	store := &statefulRoomStore{states: make(map[string]PersistentState)}
	manager := NewManagerWithStore(score.NewService(nil), store)
	actor, err := manager.Create(ctx, testConfig(), Identity{ID: "u1", Name: "小北"})
	if err != nil {
		t.Fatal(err)
	}
	if err := actor.BroadcastScoreAddition(ctx, "u1", "add-before-restart", 250, 1250); err != nil {
		t.Fatal(err)
	}
	epoch := score.Epoch{ID: 2, BaseScore: 1000, Reason: "测试重置", CreatedAt: time.Now().UTC()}
	if err := actor.BroadcastGlobalReset(ctx, epoch, "reset-before-restart"); err != nil {
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
		t.Fatal("room disappeared after restart")
	}
	snapshot, err := restoredActor.Snapshot(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	scoreMessages := 0
	for _, message := range snapshot.Messages {
		if message.Type == "score" {
			scoreMessages++
		}
	}
	if scoreMessages != 2 {
		t.Fatalf("score/reset messages lost after restart: %#v", snapshot.Messages)
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

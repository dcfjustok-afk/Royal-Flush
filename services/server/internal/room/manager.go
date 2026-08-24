package room

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

var (
	ErrAlreadySeated        = errors.New("an account can occupy a seat in only one room")
	ErrRoomLeaseUnavailable = errors.New("room lease is already owned by another server")
)

type Manager struct {
	mu             sync.RWMutex
	transitionMu   sync.Mutex
	scores         AccountScores
	store          Store
	lease          Lease
	instanceID     string
	leaseTTL       time.Duration
	leaseRenew     time.Duration
	leaseStops     map[string]chan struct{}
	rooms          map[string]*Actor
	byCode         map[string]*Actor
	activeSeat     map[string]string
	emptyTimers    map[string]*time.Timer
	emptyWait      time.Duration
	onSeatReleased func(string)
}

func (m *Manager) SetSeatReleasedHook(hook func(string)) {
	m.mu.Lock()
	m.onSeatReleased = hook
	m.mu.Unlock()
}

func NewManager(scores AccountScores) *Manager {
	return NewManagerWithStore(scores, nil)
}

func NewManagerWithStore(scores AccountScores, store Store) *Manager {
	return NewManagerWithInfrastructure(scores, store, nil, "")
}

func NewManagerWithInfrastructure(scores AccountScores, store Store, lease Lease, instanceID string) *Manager {
	return &Manager{
		scores: scores, store: store, lease: lease, instanceID: instanceID,
		leaseTTL: 15 * time.Second, leaseRenew: 5 * time.Second, leaseStops: make(map[string]chan struct{}),
		rooms: make(map[string]*Actor), byCode: make(map[string]*Actor), activeSeat: make(map[string]string),
		emptyTimers: make(map[string]*time.Timer), emptyWait: 30 * time.Minute,
	}
}

func (m *Manager) Create(ctx context.Context, config Config, owner Identity) (*Actor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeSeat[owner.ID] != "" {
		return nil, ErrAlreadySeated
	}
	actor, err := NewActor(config, owner, m.scores, m.releaseSeat)
	if err != nil {
		return nil, err
	}
	if m.lease != nil {
		acquired, err := m.lease.Acquire(ctx, actor.ID, m.instanceID, m.leaseTTL)
		if err != nil {
			actor.Close()
			return nil, err
		}
		if !acquired {
			actor.Close()
			return nil, ErrRoomLeaseUnavailable
		}
	}
	if m.store != nil {
		ownerPlayer := actor.game.Seats[0]
		err := m.store.CreateRoom(ctx, Record{
			ID: actor.ID, Code: actor.Code, OwnerID: actor.OwnerID, Config: config,
			Version: actor.version, CreatedAt: actor.CreatedAt,
		}, SeatRecord{
			ID: ownerPlayer.SeatSessionID, RoomID: actor.ID, UserID: ownerPlayer.UserID, Seat: ownerPlayer.Seat,
			AllocatedPoints: ownerPlayer.Allocated, JoinedAt: actor.CreatedAt,
		})
		if err != nil {
			m.releaseLease(actor.ID)
			actor.Close()
			return nil, err
		}
	}
	m.configureActor(actor)
	if stateStore, ok := m.store.(StateStore); ok {
		state, err := actor.PersistentState(ctx)
		if err == nil {
			err = stateStore.SaveRoomState(ctx, state)
		}
		if err != nil {
			_ = m.store.EndRoom(context.Background(), actor.ID, time.Now().UTC())
			m.releaseLease(actor.ID)
			actor.Close()
			return nil, err
		}
	}
	m.rooms[actor.ID] = actor
	m.byCode[actor.Code] = actor
	m.activeSeat[owner.ID] = actor.ID
	m.startLeaseRenewal(actor)
	return actor, nil
}

func (m *Manager) configureActor(actor *Actor) {
	actor.onCodeChanged = func(oldCode, newCode string) error { return m.updateRoomCode(actor, oldCode, newCode) }
	actor.onOwnerChanged = func(ownerID string) error {
		if m.store == nil {
			return nil
		}
		return m.store.UpdateRoomOwner(context.Background(), actor.ID, ownerID)
	}
	actor.onRoomEnded = func() error {
		if m.store == nil {
			return nil
		}
		return m.store.EndRoom(context.Background(), actor.ID, time.Now().UTC())
	}
	actor.onSeatOpened = func(seat SeatRecord, claimOwnership bool) error {
		if m.store == nil {
			return nil
		}
		return m.store.OpenSeat(context.Background(), seat, claimOwnership)
	}
	actor.onJoin = func(seat SeatRecord, claimOwnership bool, actorUserID string, event Envelope, state PersistentState) error {
		if m.store == nil {
			return nil
		}
		if atomicStore, ok := m.store.(AtomicJoinStore); ok {
			return atomicStore.OpenSeatAndAppendRoomEventAndState(context.Background(), seat, claimOwnership, actorUserID, event, state)
		}
		if err := m.store.OpenSeat(context.Background(), seat, claimOwnership); err != nil {
			return err
		}
		return m.persistEvent(actorUserID, event, state)
	}
	actor.onSeatRefilled = func(seatSessionID string, amount int64) error {
		if m.store == nil {
			return nil
		}
		return m.store.AddSeatAllocation(context.Background(), seatSessionID, amount)
	}
	actor.onEvent = func(actorUserID string, event Envelope, state PersistentState) error {
		return m.persistEvent(actorUserID, event, state)
	}
}

func (m *Manager) persistEvent(actorUserID string, event Envelope, state PersistentState) error {
	if m.store == nil {
		return nil
	}
	if atomicStore, ok := m.store.(AtomicStateStore); ok {
		return atomicStore.AppendRoomEventAndState(context.Background(), actorUserID, event, state)
	}
	if err := m.store.AppendRoomEvent(context.Background(), actorUserID, event); err != nil {
		return err
	}
	if stateStore, ok := m.store.(StateStore); ok {
		return stateStore.SaveRoomState(context.Background(), state)
	}
	return nil
}

func (m *Manager) Restore(ctx context.Context) error {
	stateStore, ok := m.store.(StateStore)
	if !ok {
		return nil
	}
	states, err := stateStore.LoadRoomStates(ctx)
	if err != nil {
		return err
	}
	for _, state := range states {
		actor, err := NewActorFromState(state, m.scores, m.releaseSeat)
		if err != nil {
			return err
		}
		if m.lease != nil {
			acquired, err := m.lease.Acquire(ctx, actor.ID, m.instanceID, m.leaseTTL)
			if err != nil {
				actor.Close()
				return err
			}
			if !acquired {
				actor.Close()
				continue
			}
		}
		m.configureActor(actor)
		m.mu.Lock()
		if m.rooms[actor.ID] != nil || m.byCode[actor.Code] != nil {
			m.mu.Unlock()
			m.releaseLease(actor.ID)
			actor.Close()
			return errors.New("duplicate persisted room identity")
		}
		for _, player := range actor.game.Seats {
			if player == nil {
				continue
			}
			if m.activeSeat[player.UserID] != "" {
				m.mu.Unlock()
				m.releaseLease(actor.ID)
				actor.Close()
				return ErrAlreadySeated
			}
			m.activeSeat[player.UserID] = actor.ID
		}
		m.rooms[actor.ID] = actor
		m.byCode[actor.Code] = actor
		m.startLeaseRenewal(actor)
		m.mu.Unlock()
		if err := actor.ResumeAfterRestart(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (m *Manager) Join(ctx context.Context, roomID string, identity Identity, seat int) (TableSnapshot, error) {
	m.transitionMu.Lock()
	defer m.transitionMu.Unlock()
	m.mu.Lock()
	existingRoomID := m.activeSeat[identity.ID]
	actor := m.rooms[roomID]
	if actor == nil {
		actor = m.byCode[roomID]
	}
	if actor == nil {
		m.mu.Unlock()
		return TableSnapshot{}, errors.New("room not found")
	}
	if existingRoomID == actor.ID {
		m.mu.Unlock()
		return actor.Snapshot(ctx, identity.ID)
	}
	var current *Actor
	if existingRoomID != "" {
		current = m.rooms[existingRoomID]
	}
	m.mu.Unlock()
	if current != nil {
		previousSeat, err := current.SeatForImmediateSwitch(ctx, identity.ID)
		if err != nil {
			return TableSnapshot{}, err
		}
		if err := actor.ReserveSeatForSwitch(ctx, identity.ID, seat); err != nil {
			return TableSnapshot{}, err
		}
		defer actor.ReleaseSeatReservation(identity.ID, seat)
		requestID, err := idgen.ID("switch")
		if err != nil {
			return TableSnapshot{}, err
		}
		if _, _, err := current.Handle(ctx, identity.ID, ClientCommand{Type: "room.leave", RequestID: requestID}); err != nil {
			return TableSnapshot{}, err
		}
		m.mu.Lock()
		m.activeSeat[identity.ID] = actor.ID
		m.cancelEmptyTimer(actor.ID)
		m.mu.Unlock()
		snapshot, err := actor.Join(ctx, identity, seat)
		if err == nil {
			return snapshot, nil
		}
		m.releaseSeat(identity.ID)
		m.mu.Lock()
		m.activeSeat[identity.ID] = current.ID
		m.cancelEmptyTimer(current.ID)
		m.mu.Unlock()
		if _, restoreErr := current.Join(ctx, identity, previousSeat); restoreErr != nil {
			m.releaseSeat(identity.ID)
			return TableSnapshot{}, fmt.Errorf("join target room: %w; restore previous room: %v", err, restoreErr)
		}
		return TableSnapshot{}, err
	}
	m.mu.Lock()
	m.activeSeat[identity.ID] = actor.ID
	m.cancelEmptyTimer(actor.ID)
	m.mu.Unlock()
	snapshot, err := actor.Join(ctx, identity, seat)
	if err != nil {
		m.releaseSeat(identity.ID)
		return TableSnapshot{}, err
	}
	return snapshot, nil
}

func (m *Manager) Room(idOrCode string) (*Actor, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	actor := m.rooms[idOrCode]
	if actor == nil {
		actor = m.byCode[idOrCode]
	}
	return actor, actor != nil
}

func (m *Manager) ActiveRoom(userID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeSeat[userID]
}

type AdminRoom struct {
	ID            string    `json:"id"`
	Code          string    `json:"code"`
	Name          string    `json:"name"`
	OwnerID       string    `json:"ownerId"`
	OwnerName     string    `json:"ownerName"`
	Players       int       `json:"players"`
	OnlinePlayers int       `json:"onlinePlayers"`
	MaxPlayers    int       `json:"maxPlayers"`
	BlindPreset   string    `json:"blindPreset"`
	HandNumber    int64     `json:"handNumber"`
	VoiceEnabled  bool      `json:"voiceEnabled"`
	Status        string    `json:"status"`
	Version       int64     `json:"version"`
	CreatedAt     time.Time `json:"createdAt"`
}

func (m *Manager) AdminRooms(ctx context.Context) []AdminRoom {
	m.mu.RLock()
	actors := make([]*Actor, 0, len(m.rooms))
	for _, actor := range m.rooms {
		actors = append(actors, actor)
	}
	m.mu.RUnlock()
	result := make([]AdminRoom, 0, len(actors))
	for _, actor := range actors {
		snapshot, err := actor.PublicSnapshot(ctx)
		if err != nil {
			continue
		}
		status := "waiting"
		if snapshot.Street == "preflop" || snapshot.Street == "flop" || snapshot.Street == "turn" || snapshot.Street == "river" || snapshot.Street == "showdown" {
			status = "playing"
		}
		if snapshot.Ended {
			status = "ended"
		}
		online := 0
		owner := ""
		for _, player := range snapshot.Players {
			if player.Status != "disconnected" {
				online++
			}
			if player.ID == snapshot.OwnerID {
				owner = player.Name
			}
		}
		result = append(result, AdminRoom{
			ID: snapshot.RoomID, Code: snapshot.RoomCode, Name: snapshot.RoomName, OwnerID: snapshot.OwnerID, OwnerName: owner,
			Players: len(snapshot.Players), OnlinePlayers: online, MaxPlayers: snapshot.Config.MaxPlayers,
			BlindPreset: snapshot.Config.BlindPreset, HandNumber: snapshot.HandNumber, VoiceEnabled: snapshot.Config.VoiceEnabled,
			Status: status, Version: snapshot.Version, CreatedAt: actor.CreatedAt,
		})
	}
	return result
}

func (m *Manager) BroadcastGlobalReset(ctx context.Context, epoch score.Epoch, requestID string) {
	m.mu.RLock()
	rooms := make([]*Actor, 0, len(m.rooms))
	for _, actor := range m.rooms {
		rooms = append(rooms, actor)
	}
	m.mu.RUnlock()
	for _, actor := range rooms {
		_ = actor.BroadcastGlobalReset(ctx, epoch, requestID)
	}
}

func (m *Manager) releaseSeat(userID string) {
	m.mu.Lock()
	roomID := m.activeSeat[userID]
	delete(m.activeSeat, userID)
	if roomID != "" && !m.hasActiveSeatLocked(roomID) {
		if timer := m.emptyTimers[roomID]; timer != nil {
			timer.Stop()
		}
		actor := m.rooms[roomID]
		m.emptyTimers[roomID] = time.AfterFunc(m.emptyWait, func() { m.expireEmptyRoom(roomID, actor) })
	}
	hook := m.onSeatReleased
	m.mu.Unlock()
	if hook != nil {
		hook(userID)
	}
}

func (m *Manager) hasActiveSeatLocked(roomID string) bool {
	for _, activeRoomID := range m.activeSeat {
		if activeRoomID == roomID {
			return true
		}
	}
	return false
}

func (m *Manager) cancelEmptyTimer(roomID string) {
	if timer := m.emptyTimers[roomID]; timer != nil {
		timer.Stop()
		delete(m.emptyTimers, roomID)
	}
}

func (m *Manager) expireEmptyRoom(roomID string, actor *Actor) {
	m.mu.Lock()
	if actor == nil || m.rooms[roomID] != actor || m.hasActiveSeatLocked(roomID) {
		m.mu.Unlock()
		return
	}
	delete(m.rooms, roomID)
	if m.byCode[actor.Code] == actor {
		delete(m.byCode, actor.Code)
	}
	delete(m.emptyTimers, roomID)
	m.stopLeaseRenewalLocked(roomID)
	m.mu.Unlock()
	if m.store != nil {
		_ = m.store.EndRoom(context.Background(), roomID, time.Now().UTC())
	}
	m.releaseLease(roomID)
	actor.Close()
}

func (m *Manager) updateRoomCode(actor *Actor, oldCode, newCode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store != nil {
		if err := m.store.UpdateRoomCode(context.Background(), actor.ID, oldCode, newCode); err != nil {
			return err
		}
	}
	if m.byCode[oldCode] == actor {
		delete(m.byCode, oldCode)
	}
	m.byCode[newCode] = actor
	return nil
}

func (m *Manager) Close() {
	m.mu.Lock()
	actors := make([]*Actor, 0, len(m.rooms))
	for _, timer := range m.emptyTimers {
		timer.Stop()
	}
	for roomID := range m.leaseStops {
		m.stopLeaseRenewalLocked(roomID)
	}
	for _, actor := range m.rooms {
		actors = append(actors, actor)
	}
	m.rooms = make(map[string]*Actor)
	m.byCode = make(map[string]*Actor)
	m.activeSeat = make(map[string]string)
	m.emptyTimers = make(map[string]*time.Timer)
	m.mu.Unlock()
	for _, actor := range actors {
		m.releaseLease(actor.ID)
		actor.Close()
	}
}

func (m *Manager) startLeaseRenewal(actor *Actor) {
	if m.lease == nil {
		return
	}
	stop := make(chan struct{})
	m.leaseStops[actor.ID] = stop
	go func() {
		ticker := time.NewTicker(m.leaseRenew)
		defer ticker.Stop()
		failures := 0
		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), m.leaseRenew)
				err := m.lease.Renew(ctx, actor.ID, m.instanceID, m.leaseTTL)
				cancel()
				if err == nil {
					failures = 0
					continue
				}
				if errors.Is(err, ErrLeaseNotOwned) {
					m.expireLostLease(actor.ID, actor)
					return
				}
				failures++
				if failures >= 3 {
					m.expireLostLease(actor.ID, actor)
					return
				}
			case <-stop:
				return
			}
		}
	}()
}

func (m *Manager) expireLostLease(roomID string, actor *Actor) {
	m.mu.Lock()
	if m.rooms[roomID] != actor {
		m.mu.Unlock()
		return
	}
	delete(m.rooms, roomID)
	if m.byCode[actor.Code] == actor {
		delete(m.byCode, actor.Code)
	}
	for userID, activeRoomID := range m.activeSeat {
		if activeRoomID == roomID {
			delete(m.activeSeat, userID)
		}
	}
	if timer := m.emptyTimers[roomID]; timer != nil {
		timer.Stop()
		delete(m.emptyTimers, roomID)
	}
	delete(m.leaseStops, roomID)
	m.mu.Unlock()
	actor.Close()
}

func (m *Manager) stopLeaseRenewalLocked(roomID string) {
	if stop := m.leaseStops[roomID]; stop != nil {
		close(stop)
		delete(m.leaseStops, roomID)
	}
}

func (m *Manager) releaseLease(roomID string) {
	if m.lease == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = m.lease.Release(ctx, roomID, m.instanceID)
}

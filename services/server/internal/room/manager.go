package room

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

var (
	ErrAlreadySeated        = errors.New("an account can occupy a seat in only one room")
	ErrRoomLeaseUnavailable = errors.New("room lease is already owned by another server")
)

type Manager struct {
	mu          sync.RWMutex
	scores      AccountScores
	store       Store
	lease       Lease
	instanceID  string
	leaseTTL    time.Duration
	leaseRenew  time.Duration
	leaseStops  map[string]chan struct{}
	rooms       map[string]*Actor
	byCode      map[string]*Actor
	activeSeat  map[string]string
	emptyTimers map[string]*time.Timer
	emptyWait   time.Duration
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
		err := m.store.CreateRoom(ctx, Record{
			ID: actor.ID, Code: actor.Code, OwnerID: actor.OwnerID, Config: config,
			Version: actor.version, CreatedAt: actor.CreatedAt,
		})
		if err != nil {
			m.releaseLease(actor.ID)
			actor.Close()
			return nil, err
		}
	}
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
	m.rooms[actor.ID] = actor
	m.byCode[actor.Code] = actor
	m.activeSeat[owner.ID] = actor.ID
	m.startLeaseRenewal(actor)
	return actor, nil
}

func (m *Manager) Join(ctx context.Context, roomID string, identity Identity, seat int) (TableSnapshot, error) {
	m.mu.Lock()
	if existing := m.activeSeat[identity.ID]; existing != "" && existing != roomID {
		m.mu.Unlock()
		return TableSnapshot{}, ErrAlreadySeated
	}
	actor := m.rooms[roomID]
	if actor == nil {
		actor = m.byCode[roomID]
	}
	if actor == nil {
		m.mu.Unlock()
		return TableSnapshot{}, errors.New("room not found")
	}
	m.cancelEmptyTimer(actor.ID)
	if m.activeSeat[identity.ID] == actor.ID {
		m.mu.Unlock()
		return actor.Snapshot(ctx, identity.ID)
	}
	m.activeSeat[identity.ID] = actor.ID
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
	m.mu.Unlock()
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

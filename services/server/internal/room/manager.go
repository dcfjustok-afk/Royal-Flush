package room

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

var ErrAlreadySeated = errors.New("an account can occupy a seat in only one room")

type Manager struct {
	mu          sync.RWMutex
	scores      AccountScores
	rooms       map[string]*Actor
	byCode      map[string]*Actor
	activeSeat  map[string]string
	emptyTimers map[string]*time.Timer
	emptyWait   time.Duration
}

func NewManager(scores AccountScores) *Manager {
	return &Manager{scores: scores, rooms: make(map[string]*Actor), byCode: make(map[string]*Actor), activeSeat: make(map[string]string), emptyTimers: make(map[string]*time.Timer), emptyWait: 30 * time.Minute}
}

func (m *Manager) Create(_ context.Context, config Config, owner Identity) (*Actor, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.activeSeat[owner.ID] != "" {
		return nil, ErrAlreadySeated
	}
	actor, err := NewActor(config, owner, m.scores, m.releaseSeat)
	if err != nil {
		return nil, err
	}
	actor.onCodeChanged = func(oldCode, newCode string) {
		m.updateRoomCode(actor, oldCode, newCode)
	}
	m.rooms[actor.ID] = actor
	m.byCode[actor.Code] = actor
	m.activeSeat[owner.ID] = actor.ID
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
	m.mu.Unlock()
	actor.Close()
}

func (m *Manager) updateRoomCode(actor *Actor, oldCode, newCode string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.byCode[oldCode] == actor {
		delete(m.byCode, oldCode)
	}
	m.byCode[newCode] = actor
}

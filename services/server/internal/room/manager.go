package room

import (
	"context"
	"errors"
	"sync"

	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

var ErrAlreadySeated = errors.New("an account can occupy a seat in only one room")

type Manager struct {
	mu         sync.RWMutex
	scores     AccountScores
	rooms      map[string]*Actor
	byCode     map[string]*Actor
	activeSeat map[string]string
}

func NewManager(scores AccountScores) *Manager {
	return &Manager{scores: scores, rooms: make(map[string]*Actor), byCode: make(map[string]*Actor), activeSeat: make(map[string]string)}
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
	delete(m.activeSeat, userID)
	m.mu.Unlock()
}

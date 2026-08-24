package auth

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrUserNotFound = errors.New("user not found")

type StoredUser struct {
	User
	PasswordHash string
}

type Store interface {
	SaveUser(ctx context.Context, user StoredUser) error
	UserByPhone(ctx context.Context, phone string) (StoredUser, bool, error)
	UserByID(ctx context.Context, userID string) (StoredUser, bool, error)
	SaveSession(ctx context.Context, tokenHash, userID string, expiresAt time.Time) error
	UserBySession(ctx context.Context, tokenHash string, now time.Time) (StoredUser, bool, error)
	DeleteSession(ctx context.Context, tokenHash string) error
	SetBanned(ctx context.Context, userID string, banned bool) error
}

type memorySession struct {
	userID    string
	expiresAt time.Time
}

type MemoryStore struct {
	mu           sync.Mutex
	usersByID    map[string]StoredUser
	usersByPhone map[string]string
	sessions     map[string]memorySession
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{usersByID: map[string]StoredUser{}, usersByPhone: map[string]string{}, sessions: map[string]memorySession{}}
}

func (s *MemoryStore) SaveUser(_ context.Context, record StoredUser) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.Phone != "" {
		if existingID := s.usersByPhone[record.Phone]; existingID != "" && existingID != record.ID {
			return ErrPhoneRegistered
		}
	}
	if existing, ok := s.usersByID[record.ID]; ok && record.PasswordHash == "" {
		record.PasswordHash = existing.PasswordHash
	}
	record.User = cloneUser(&record.User)
	s.usersByID[record.ID] = record
	if record.Phone != "" {
		s.usersByPhone[record.Phone] = record.ID
	}
	return nil
}

func (s *MemoryStore) UserByPhone(_ context.Context, phone string) (StoredUser, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.usersByID[s.usersByPhone[phone]]
	return cloneStoredUser(record), ok, nil
}

func (s *MemoryStore) UserByID(_ context.Context, userID string) (StoredUser, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.usersByID[userID]
	return cloneStoredUser(record), ok, nil
}

func (s *MemoryStore) SaveSession(_ context.Context, tokenHash, userID string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.usersByID[userID]; !ok {
		return ErrUserNotFound
	}
	s.sessions[tokenHash] = memorySession{userID: userID, expiresAt: expiresAt}
	return nil
}

func (s *MemoryStore) UserBySession(_ context.Context, tokenHash string, now time.Time) (StoredUser, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[tokenHash]
	if !ok || !now.Before(session.expiresAt) {
		delete(s.sessions, tokenHash)
		return StoredUser{}, false, nil
	}
	record, ok := s.usersByID[session.userID]
	return cloneStoredUser(record), ok, nil
}

func (s *MemoryStore) DeleteSession(_ context.Context, tokenHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, tokenHash)
	return nil
}

func (s *MemoryStore) SetBanned(_ context.Context, userID string, banned bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.usersByID[userID]
	if !ok {
		return ErrUserNotFound
	}
	record.Banned = banned
	s.usersByID[userID] = record
	return nil
}

func cloneStoredUser(record StoredUser) StoredUser {
	record.User = cloneUser(&record.User)
	return record
}

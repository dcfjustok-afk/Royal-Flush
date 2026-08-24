package score

import (
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	InitialBalance  int64 = 1000
	MaximumAddition int64 = 1_000_000_000
)

var (
	ErrInvalidAmount = errors.New("amount must be an integer between 1 and 1,000,000,000")
	ErrRateLimited   = errors.New("score addition is limited to once every 5 seconds")
	ErrRequestID     = errors.New("requestId is required")
	ErrSessionID     = errors.New("seat session id is required")
)

type EntryType string

const (
	EntryInitial    EntryType = "initial_base"
	EntrySelfAdd    EntryType = "self_add"
	EntrySettlement EntryType = "game_settlement"
	EntryReset      EntryType = "admin_reset"
)

type LedgerEntry struct {
	ID        string    `json:"id"`
	Type      EntryType `json:"type"`
	Amount    int64     `json:"amount"`
	Balance   int64     `json:"balance"`
	RoomID    string    `json:"roomId,omitempty"`
	Note      string    `json:"note"`
	CreatedAt time.Time `json:"createdAt"`
	EpochID   int64     `json:"epochId"`
	RequestID string    `json:"requestId,omitempty"`
}

type Epoch struct {
	ID            int64     `json:"id"`
	BaseScore     int64     `json:"baseScore"`
	Administrator string    `json:"administrator"`
	Reason        string    `json:"reason"`
	AffectedUsers int       `json:"affectedUsers"`
	CreatedAt     time.Time `json:"createdAt"`
}

type Result struct {
	Balance int64       `json:"balance"`
	Entry   LedgerEntry `json:"entry"`
}

type Service struct {
	mu                sync.Mutex
	now               func() time.Time
	store             Store
	nextID            int64
	current           Epoch
	epochs            []Epoch
	users             map[string]time.Time
	deltas            map[string]map[int64]int64
	entries           map[string][]LedgerEntry
	additionResults   map[string]Result
	settlementResults map[string]Result
	resetResults      map[string]Epoch
	lastAddition      map[string]time.Time
}

func NewService(now func() time.Time) *Service {
	return NewServiceWithStore(nil, now)
}

func NewServiceWithStore(store Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	createdAt := now().UTC()
	initial := Epoch{ID: 1, BaseScore: InitialBalance, Administrator: "system", Reason: "initial epoch", CreatedAt: createdAt}
	return &Service{
		now:               now,
		store:             store,
		nextID:            1,
		current:           initial,
		epochs:            []Epoch{initial},
		users:             make(map[string]time.Time),
		deltas:            make(map[string]map[int64]int64),
		entries:           make(map[string][]LedgerEntry),
		additionResults:   make(map[string]Result),
		settlementResults: make(map[string]Result),
		resetResults:      make(map[string]Epoch),
		lastAddition:      make(map[string]time.Time),
	}
}

func (s *Service) EnsureUser(userID string) int64 {
	if s.store != nil {
		balance, err := s.store.EnsureUser(userID, s.now().UTC())
		if err == nil {
			return balance
		}
		return InitialBalance
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureUserLocked(userID)
	return s.balanceLocked(userID)
}

func (s *Service) Balance(userID string) int64 {
	if s.store != nil {
		balance, err := s.store.Balance(userID)
		if err == nil {
			return balance
		}
		return InitialBalance
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureUserLocked(userID)
	return s.balanceLocked(userID)
}

func (s *Service) Add(userID, roomID, requestID string, amount int64) (Result, error) {
	if requestID == "" {
		return Result{}, ErrRequestID
	}
	if amount < 1 || amount > MaximumAddition {
		return Result{}, ErrInvalidAmount
	}
	if s.store != nil {
		return s.store.Add(userID, roomID, requestID, amount, s.now().UTC())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := userID + "\x00" + requestID
	if result, ok := s.additionResults[key]; ok {
		return result, nil
	}
	now := s.now().UTC()
	if last, ok := s.lastAddition[userID]; ok && now.Sub(last) < 5*time.Second {
		return Result{}, ErrRateLimited
	}
	s.ensureUserLocked(userID)
	s.deltas[userID][s.current.ID] += amount
	entry := s.newEntryLocked(userID, EntrySelfAdd, amount, roomID, "自行增加积分", requestID, now)
	result := Result{Balance: entry.Balance, Entry: entry}
	s.additionResults[key] = result
	s.lastAddition[userID] = now
	return result, nil
}

func (s *Service) ApplySettlement(userID, roomID, seatSessionID string, net int64) (Result, error) {
	if seatSessionID == "" {
		return Result{}, ErrSessionID
	}
	if s.store != nil {
		return s.store.ApplySettlement(userID, roomID, seatSessionID, net, s.now().UTC())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if result, ok := s.settlementResults[seatSessionID]; ok {
		return result, nil
	}
	s.ensureUserLocked(userID)
	s.deltas[userID][s.current.ID] += net
	entry := s.newEntryLocked(userID, EntrySettlement, net, roomID, "牌局净输赢结算", seatSessionID, s.now().UTC())
	result := Result{Balance: entry.Balance, Entry: entry}
	s.settlementResults[seatSessionID] = result
	return result, nil
}

func (s *Service) ResetAll(administrator, reason string) (Epoch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.resetAllLocked(administrator, reason)
}

func (s *Service) ResetAllWithRequest(administrator, reason, requestID string) (Epoch, bool, error) {
	if requestID == "" {
		return Epoch{}, false, ErrRequestID
	}
	if s.store != nil {
		return s.store.ResetAllWithRequest(administrator, reason, requestID, s.now().UTC())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := administrator + "\x00" + requestID
	if epoch, ok := s.resetResults[key]; ok {
		return epoch, true, nil
	}
	epoch, err := s.resetAllLocked(administrator, reason)
	if err != nil {
		return Epoch{}, false, err
	}
	s.resetResults[key] = epoch
	return epoch, false, nil
}

func (s *Service) resetAllLocked(administrator, reason string) (Epoch, error) {
	if administrator == "" {
		return Epoch{}, errors.New("administrator is required")
	}
	if reason == "" {
		return Epoch{}, errors.New("reset reason is required")
	}
	s.nextID++
	epoch := Epoch{
		ID:            s.nextID,
		BaseScore:     InitialBalance,
		Administrator: administrator,
		Reason:        reason,
		AffectedUsers: len(s.users),
		CreatedAt:     s.now().UTC(),
	}
	s.current = epoch
	s.epochs = append(s.epochs, epoch)
	return epoch, nil
}

func (s *Service) Ledger(userID string) (int64, []LedgerEntry) {
	if s.store != nil {
		balance, entries, err := s.store.Ledger(userID)
		if err == nil {
			return balance, entries
		}
		return InitialBalance, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureUserLocked(userID)
	entries := append([]LedgerEntry(nil), s.entries[userID]...)
	if len(s.epochs) > 1 {
		current := s.current
		entries = append(entries, LedgerEntry{
			ID:        fmt.Sprintf("epoch-%d-%s", current.ID, userID),
			Type:      EntryReset,
			Amount:    current.BaseScore,
			Balance:   current.BaseScore,
			Note:      "平台管理员重置积分周期：" + current.Reason,
			CreatedAt: current.CreatedAt,
			EpochID:   current.ID,
		})
	}
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].CreatedAt.After(entries[j].CreatedAt) })
	return s.balanceLocked(userID), entries
}

func (s *Service) Epochs() []Epoch {
	if s.store != nil {
		epochs, err := s.store.Epochs()
		if err == nil {
			return epochs
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	result := append([]Epoch(nil), s.epochs...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID > result[j].ID })
	return result
}

func (s *Service) ensureUserLocked(userID string) {
	if _, ok := s.users[userID]; ok {
		if s.deltas[userID] == nil {
			s.deltas[userID] = make(map[int64]int64)
		}
		return
	}
	now := s.now().UTC()
	s.users[userID] = now
	s.deltas[userID] = make(map[int64]int64)
	s.entries[userID] = append(s.entries[userID], LedgerEntry{
		ID: fmt.Sprintf("initial-%s", userID), Type: EntryInitial, Amount: s.current.BaseScore,
		Balance: s.current.BaseScore, Note: "初始娱乐积分", CreatedAt: now, EpochID: s.current.ID,
	})
}

func (s *Service) balanceLocked(userID string) int64 {
	return s.current.BaseScore + s.deltas[userID][s.current.ID]
}

func (s *Service) newEntryLocked(userID string, typ EntryType, amount int64, roomID, note, requestID string, now time.Time) LedgerEntry {
	entry := LedgerEntry{
		ID:   fmt.Sprintf("score-%d-%s", len(s.entries[userID])+1, userID),
		Type: typ, Amount: amount, Balance: s.balanceLocked(userID), RoomID: roomID,
		Note: note, CreatedAt: now, EpochID: s.current.ID, RequestID: requestID,
	}
	s.entries[userID] = append(s.entries[userID], entry)
	return entry
}

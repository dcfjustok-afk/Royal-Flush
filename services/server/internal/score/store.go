package score

import "time"

// Store is the durable score ledger used outside the in-memory development mode.
// Implementations must make Add, ApplySettlement, and ResetAllWithRequest idempotent.
type Store interface {
	EnsureUser(userID string, now time.Time) (int64, error)
	Balance(userID string) (int64, error)
	Add(userID, roomID, requestID string, amount int64, now time.Time) (Result, error)
	ApplySettlement(userID, roomID, seatSessionID string, net int64, now time.Time) (Result, error)
	ResetAllWithRequest(administrator, reason, requestID string, now time.Time) (Epoch, bool, error)
	Ledger(userID string) (int64, []LedgerEntry, error)
	Epochs() ([]Epoch, error)
}

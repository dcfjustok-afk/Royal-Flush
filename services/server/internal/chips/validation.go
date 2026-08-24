package chips

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidChip     = errors.New("chip is not enabled for this room")
	ErrRaiseTooSmall   = errors.New("raise is below the minimum increment")
	ErrRaiseTooLarge   = errors.New("raise exceeds the player's table points")
	ErrEmptyChipList   = errors.New("at least one chip is required")
	ErrChipSumOverflow = errors.New("chip sum overflows int64")
)

type RaiseResult struct {
	RaiseBy    int64 `json:"raiseBy"`
	RaiseTo    int64 `json:"raiseTo"`
	ActionCost int64 `json:"actionCost"`
	Remaining  int64 `json:"remainingTablePoints"`
}

func ValidateRaise(values, allowed []int64, toCall, minimumRaiseBy, stack int64) (RaiseResult, error) {
	if len(values) == 0 {
		return RaiseResult{}, ErrEmptyChipList
	}
	enabled := make(map[int64]bool, len(allowed))
	for _, chip := range allowed {
		enabled[chip] = true
	}
	var raiseBy int64
	for _, chip := range values {
		if chip <= 0 || !enabled[chip] {
			return RaiseResult{}, fmt.Errorf("%w: %d", ErrInvalidChip, chip)
		}
		if raiseBy > math.MaxInt64-chip {
			return RaiseResult{}, ErrChipSumOverflow
		}
		raiseBy += chip
	}
	if raiseBy < minimumRaiseBy {
		return RaiseResult{}, fmt.Errorf("%w: missing %d", ErrRaiseTooSmall, minimumRaiseBy-raiseBy)
	}
	if toCall < 0 || stack < 0 || raiseBy > stack || toCall > stack-raiseBy {
		return RaiseResult{}, ErrRaiseTooLarge
	}
	return RaiseResult{
		RaiseBy: raiseBy, RaiseTo: toCall + raiseBy, ActionCost: toCall + raiseBy,
		Remaining: stack - toCall - raiseBy,
	}, nil
}

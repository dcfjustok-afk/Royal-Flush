package chips

import (
	"errors"
	"math"
	"reflect"
	"testing"
)

func TestValidateRaise(t *testing.T) {
	allowed := []int64{5, 10, 20, 50, 100}
	result, err := ValidateRaise([]int64{20, 5}, allowed, 10, 20, 1000)
	if err != nil {
		t.Fatal(err)
	}
	expected := RaiseResult{RaiseBy: 25, RaiseTo: 35, ActionCost: 35, Remaining: 965}
	if !reflect.DeepEqual(result, expected) {
		t.Fatalf("unexpected result: %#v", result)
	}
	result, err = ValidateRaise([]int64{100, 100}, allowed, 10, 20, 1000)
	if err != nil || result.RaiseBy != 200 {
		t.Fatalf("repeatable chips should be valid: %#v, %v", result, err)
	}
	if _, err := ValidateRaise([]int64{200}, allowed, 10, 20, 1000); !errors.Is(err, ErrInvalidChip) {
		t.Fatalf("expected invalid chip, got %v", err)
	}
	if _, err := ValidateRaise([]int64{5, 10}, allowed, 10, 20, 1000); !errors.Is(err, ErrRaiseTooSmall) {
		t.Fatalf("expected minimum raise error, got %v", err)
	}
	if _, err := ValidateRaise([]int64{100}, allowed, 950, 20, 1000); !errors.Is(err, ErrRaiseTooLarge) {
		t.Fatalf("expected stack error, got %v", err)
	}
	if _, err := ValidateRaise([]int64{math.MaxInt64, 5}, []int64{math.MaxInt64, 5}, 0, 1, math.MaxInt64); !errors.Is(err, ErrChipSumOverflow) {
		t.Fatalf("expected overflow error, got %v", err)
	}
}

package idgen

import (
	"regexp"
	"testing"
)

func TestRoomCodeIsReadableAndPrefixed(t *testing.T) {
	pattern := regexp.MustCompile(`^RF-[A-HJ-NP-Z2-9]{4}$`)
	seen := make(map[string]bool)
	for range 100 {
		code, err := RoomCode()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(code) {
			t.Fatalf("unexpected room code %q", code)
		}
		seen[code] = true
	}
	if len(seen) < 95 {
		t.Fatalf("room codes show excessive collisions: %d unique", len(seen))
	}
}

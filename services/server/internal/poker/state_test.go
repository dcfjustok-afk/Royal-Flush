package poker

import (
	"reflect"
	"testing"
)

func TestGameStateRoundTripPreservesPrivateCardsAndActionState(t *testing.T) {
	game, err := NewGame(4, 5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := game.Sit("u1", "一号", 0, "seat-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := game.Sit("u2", "二号", 2, "seat-2"); err != nil {
		t.Fatal(err)
	}
	game.Seats[0].Ready = true
	game.Seats[2].Ready = true
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	state := game.ExportState()
	restored, err := RestoreState(state)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored.ExportState(), state) {
		t.Fatalf("restored state differs from source\nsource: %#v\nrestored: %#v", state, restored.ExportState())
	}
}

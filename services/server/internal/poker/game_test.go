package poker

import (
	"testing"

	"github.com/cardrank/cardrank"
)

func TestAllStandardHandCategories(t *testing.T) {
	tests := []struct {
		name   string
		pocket []Card
		board  []Card
	}{
		{"straight flush", cards("Ah", "Kh"), cards("Qh", "Jh", "10h", "2c", "3d")},
		{"four of a kind", cards("Ah", "Ad"), cards("Ac", "As", "2d", "3c", "4c")},
		{"full house", cards("Ah", "Ad"), cards("Ac", "Ks", "Kd", "3c", "4c")},
		{"flush", cards("Ah", "2h"), cards("Kh", "Qh", "9h", "3c", "4d")},
		{"straight", cards("Ah", "Kd"), cards("Qc", "Js", "10h", "3c", "4d")},
		{"three of a kind", cards("Ah", "Ad"), cards("Ac", "Ks", "Qd", "3c", "4c")},
		{"two pair", cards("Ah", "Ad"), cards("Kc", "Ks", "2d", "3c", "4c")},
		{"one pair", cards("Ah", "Ad"), cards("Kc", "Qs", "2d", "3c", "4c")},
		{"high card", cards("Ah", "Kd"), cards("Qc", "9s", "7h", "3d", "2c")},
	}
	var previous cardrank.EvalRank
	for index, test := range tests {
		evaluation, err := Evaluate(test.pocket, test.board)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if index > 0 && evaluation.HiRank <= previous {
			t.Fatalf("%s rank %d should be worse than previous rank %d", test.name, evaluation.HiRank, previous)
		}
		previous = evaluation.HiRank
	}
}

func TestHeadsUpBlindsAndActionOrder(t *testing.T) {
	game, _ := NewGame(8, 5, 10)
	_, _ = game.Sit("u1", "一号", 0, "s1")
	_, _ = game.Sit("u2", "二号", 4, "s2")
	game.Seats[0].Ready = true
	game.Seats[4].Ready = true
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	if game.Button != 0 || game.SmallBlindSeat != 0 || game.BigBlindSeat != 4 || game.Actor != 0 {
		t.Fatalf("incorrect heads-up positions: button=%d sb=%d bb=%d actor=%d", game.Button, game.SmallBlindSeat, game.BigBlindSeat, game.Actor)
	}
	if len(game.Seats[0].Hole) != 2 || len(game.Seats[4].Hole) != 2 {
		t.Fatal("players did not receive exactly two cards")
	}
}

func TestStartHandExcludesUnreadyAndDisconnectedPlayers(t *testing.T) {
	game, _ := NewGame(4, 5, 10)
	_, _ = game.Sit("owner", "房主", 0, "s1")
	_, _ = game.Sit("ready", "已准备", 1, "s2")
	_, _ = game.Sit("unready", "未准备", 2, "s3")
	_, _ = game.Sit("offline", "已断线", 3, "s4")
	game.Seats[0].Ready = true
	game.Seats[1].Ready = true
	game.Seats[3].Ready = true
	game.Seats[3].Disconnected = true

	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	if len(game.Seats[0].Hole) != 2 || len(game.Seats[1].Hole) != 2 {
		t.Fatal("ready connected players did not receive cards")
	}
	for _, seat := range []int{2, 3} {
		if len(game.Seats[seat].Hole) != 0 || game.Seats[seat].HandCommitted != 0 {
			t.Fatalf("ineligible seat %d joined the hand: %#v", seat, game.Seats[seat])
		}
	}
}

func TestFoldAwardsPotOnlyToAPlayerDealtIntoTheHand(t *testing.T) {
	game, _ := NewGame(3, 5, 10)
	_, _ = game.Sit("spectator", "旁观者", 0, "s1")
	_, _ = game.Sit("ready-one", "玩家一", 1, "s2")
	_, _ = game.Sit("ready-two", "玩家二", 2, "s3")
	game.Seats[1].Ready = true
	game.Seats[2].Ready = true
	if err := game.StartHand(); err != nil {
		t.Fatal(err)
	}
	actor := game.Actor
	winner := map[int]int{1: 2, 2: 1}[actor]
	if err := game.Fold(actor); err != nil {
		t.Fatal(err)
	}
	if game.Seats[0].Stack != 1000 {
		t.Fatalf("spectator received the pot: stack=%d", game.Seats[0].Stack)
	}
	if game.Seats[winner].Stack <= 1000 {
		t.Fatalf("remaining participant did not receive the pot: seat=%d stack=%d", winner, game.Seats[winner].Stack)
	}
}

func TestCumulativeShortAllInsReopenRaise(t *testing.T) {
	game, _ := NewGame(3, 5, 10)
	game.Street = StreetPreflop
	game.CurrentBet = 100
	game.MinimumRaiseBy = 100
	game.Actor = 1
	game.pending = map[int]bool{0: true, 1: true, 2: true}
	game.actedAtBet = map[int]int64{0: 100}
	for seat := range 3 {
		game.Seats[seat] = &Player{Seat: seat, UserID: string(rune('a' + seat)), Stack: 500, StreetCommitted: 100, HandCommitted: 100, Hole: cards("Ah", "Kd")}
	}
	game.Seats[1].Stack = 50
	game.Seats[2].Stack = 100
	if err := game.AllIn(1); err != nil {
		t.Fatal(err)
	}
	if game.CurrentBet != 150 || game.Actor != 2 || game.CanRaise(0) {
		t.Fatalf("first short all-in state is wrong: %s", game.DebugState())
	}
	if err := game.AllIn(2); err != nil {
		t.Fatal(err)
	}
	if game.CurrentBet != 200 || game.Actor != 0 || !game.CanRaise(0) {
		t.Fatalf("cumulative short all-ins should reopen action: %s", game.DebugState())
	}
}

func TestSidePotsAndOddPointDistribution(t *testing.T) {
	game, _ := NewGame(3, 5, 10)
	game.Button = 0
	game.Board = cards("2c", "3c", "4d", "5s", "9h")
	game.Seats[0] = &Player{Seat: 0, Hole: cards("Ah", "Ad"), HandCommitted: 100}
	game.Seats[1] = &Player{Seat: 1, Hole: cards("Kh", "Kd"), HandCommitted: 200}
	game.Seats[2] = &Player{Seat: 2, Hole: cards("Qh", "Qd"), HandCommitted: 300}
	if err := game.showdown(); err != nil {
		t.Fatal(err)
	}
	if game.Seats[0].Stack != 300 || game.Seats[1].Stack != 200 || game.Seats[2].Stack != 100 {
		t.Fatalf("incorrect side-pot payout: %d/%d/%d", game.Seats[0].Stack, game.Seats[1].Stack, game.Seats[2].Stack)
	}

	odd, _ := NewGame(3, 5, 10)
	odd.Button = 0
	odd.Board = cards("Ah", "Kh", "Qh", "Jh", "10h")
	odd.Seats[0] = &Player{Seat: 0, Hole: cards("2c", "3d"), HandCommitted: 5}
	odd.Seats[1] = &Player{Seat: 1, Hole: cards("4c", "5d"), HandCommitted: 5, Folded: true}
	odd.Seats[2] = &Player{Seat: 2, Hole: cards("6c", "7d"), HandCommitted: 5}
	if err := odd.showdown(); err != nil {
		t.Fatal(err)
	}
	if odd.Seats[2].Stack != 8 || odd.Seats[0].Stack != 7 {
		t.Fatalf("odd point should go to first winner left of button: seat0=%d seat2=%d", odd.Seats[0].Stack, odd.Seats[2].Stack)
	}
}

func cards(values ...string) []Card {
	result := make([]Card, 0, len(values))
	for _, value := range values {
		rank := value[:len(value)-1]
		suit := map[byte]string{'s': "spades", 'h': "hearts", 'd': "diamonds", 'c': "clubs"}[value[len(value)-1]]
		result = append(result, Card{Rank: rank, Suit: suit})
	}
	return result
}

package poker

import "errors"

type PlayerState struct {
	UserID          string `json:"userId"`
	Name            string `json:"name"`
	Seat            int    `json:"seat"`
	Stack           int64  `json:"tablePoints"`
	Allocated       int64  `json:"allocatedTablePoints"`
	SeatSessionID   string `json:"seatSessionId"`
	StreetCommitted int64  `json:"streetCommitted"`
	HandCommitted   int64  `json:"handCommitted"`
	Hole            []Card `json:"hole"`
	Folded          bool   `json:"folded"`
	AllIn           bool   `json:"allIn"`
	Away            bool   `json:"away"`
	Disconnected    bool   `json:"disconnected"`
	Leaving         bool   `json:"leaving"`
	Ready           bool   `json:"ready"`
}

type State struct {
	Seats          []*PlayerState `json:"seats"`
	SmallBlind     int64          `json:"smallBlind"`
	BigBlind       int64          `json:"bigBlind"`
	Button         int            `json:"button"`
	SmallBlindSeat int            `json:"smallBlindSeat"`
	BigBlindSeat   int            `json:"bigBlindSeat"`
	Actor          int            `json:"actor"`
	Street         Street         `json:"street"`
	HandNumber     int64          `json:"handNumber"`
	Board          []Card         `json:"board"`
	Deck           []Card         `json:"deck"`
	DeckIndex      int            `json:"deckIndex"`
	CurrentBet     int64          `json:"currentBet"`
	MinimumRaiseBy int64          `json:"minimumRaiseBy"`
	Pending        map[int]bool   `json:"pending"`
	ActedAtBet     map[int]int64  `json:"actedAtBet"`
	LastPayouts    []Payout       `json:"lastPayouts"`
	Version        int64          `json:"version"`
}

func (g *Game) ExportState() State {
	state := State{
		Seats: make([]*PlayerState, len(g.Seats)), SmallBlind: g.SmallBlind, BigBlind: g.BigBlind,
		Button: g.Button, SmallBlindSeat: g.SmallBlindSeat, BigBlindSeat: g.BigBlindSeat, Actor: g.Actor,
		Street: g.Street, HandNumber: g.HandNumber, Board: append([]Card(nil), g.Board...),
		Deck: append([]Card(nil), g.deck...), DeckIndex: g.deckIndex, CurrentBet: g.CurrentBet,
		MinimumRaiseBy: g.MinimumRaiseBy, Pending: cloneBoolMap(g.pending), ActedAtBet: cloneInt64Map(g.actedAtBet),
		LastPayouts: append([]Payout(nil), g.LastPayouts...), Version: g.Version,
	}
	for seat, player := range g.Seats {
		if player == nil {
			continue
		}
		state.Seats[seat] = &PlayerState{
			UserID: player.UserID, Name: player.Name, Seat: player.Seat, Stack: player.Stack,
			Allocated: player.Allocated, SeatSessionID: player.SeatSessionID,
			StreetCommitted: player.StreetCommitted, HandCommitted: player.HandCommitted,
			Hole: append([]Card(nil), player.Hole...), Folded: player.Folded, AllIn: player.AllIn,
			Away: player.Away, Disconnected: player.Disconnected, Leaving: player.Leaving, Ready: player.Ready,
		}
	}
	return state
}

func RestoreState(state State) (*Game, error) {
	if len(state.Seats) < 2 || len(state.Seats) > 8 {
		return nil, errors.New("restored game must contain between 2 and 8 seats")
	}
	if state.DeckIndex < 0 || state.DeckIndex > len(state.Deck) {
		return nil, errors.New("restored game deck index is invalid")
	}
	game := &Game{
		Seats: make([]*Player, len(state.Seats)), SmallBlind: state.SmallBlind, BigBlind: state.BigBlind,
		Button: state.Button, SmallBlindSeat: state.SmallBlindSeat, BigBlindSeat: state.BigBlindSeat,
		Actor: state.Actor, Street: state.Street, HandNumber: state.HandNumber,
		Board: append([]Card(nil), state.Board...), deck: append([]Card(nil), state.Deck...), deckIndex: state.DeckIndex,
		CurrentBet: state.CurrentBet, MinimumRaiseBy: state.MinimumRaiseBy,
		pending: cloneBoolMap(state.Pending), actedAtBet: cloneInt64Map(state.ActedAtBet),
		LastPayouts: append([]Payout(nil), state.LastPayouts...), Version: state.Version,
	}
	if game.pending == nil {
		game.pending = make(map[int]bool)
	}
	if game.actedAtBet == nil {
		game.actedAtBet = make(map[int]int64)
	}
	for seat, player := range state.Seats {
		if player == nil {
			continue
		}
		if player.Seat != seat || player.UserID == "" || player.SeatSessionID == "" {
			return nil, errors.New("restored player seat is invalid")
		}
		game.Seats[seat] = &Player{
			UserID: player.UserID, Name: player.Name, Seat: player.Seat, Stack: player.Stack,
			Allocated: player.Allocated, SeatSessionID: player.SeatSessionID,
			StreetCommitted: player.StreetCommitted, HandCommitted: player.HandCommitted,
			Hole: append([]Card(nil), player.Hole...), Folded: player.Folded, AllIn: player.AllIn,
			Away: player.Away, Disconnected: player.Disconnected, Leaving: player.Leaving, Ready: player.Ready,
		}
	}
	return game, nil
}

func cloneBoolMap(source map[int]bool) map[int]bool {
	result := make(map[int]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneInt64Map(source map[int]int64) map[int]int64 {
	result := make(map[int]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

package poker

import (
	"errors"
	"fmt"
	"sort"

	"github.com/royal-flush/royal-flush/services/server/internal/chips"
)

type Street string

const (
	StreetWaiting  Street = "waiting"
	StreetPreflop  Street = "preflop"
	StreetFlop     Street = "flop"
	StreetTurn     Street = "turn"
	StreetRiver    Street = "river"
	StreetShowdown Street = "showdown"
	StreetSettled  Street = "settled"
)

var (
	ErrNotEnoughPlayers = errors.New("at least two available players are required")
	ErrHandInProgress   = errors.New("a hand is already in progress")
	ErrNotPlayersTurn   = errors.New("it is not this player's turn")
	ErrIllegalAction    = errors.New("action is not legal in the current state")
	ErrSeatOccupied     = errors.New("seat is occupied")
	ErrPlayerSeated     = errors.New("player is already seated")
)

type Player struct {
	UserID          string `json:"userId"`
	Name            string `json:"name"`
	Seat            int    `json:"seat"`
	Stack           int64  `json:"tablePoints"`
	Allocated       int64  `json:"allocatedTablePoints"`
	SeatSessionID   string `json:"seatSessionId"`
	StreetCommitted int64  `json:"streetCommitted"`
	HandCommitted   int64  `json:"handCommitted"`
	Hole            []Card `json:"-"`
	Folded          bool   `json:"folded"`
	AllIn           bool   `json:"allIn"`
	Away            bool   `json:"away"`
	Disconnected    bool   `json:"disconnected"`
	Leaving         bool   `json:"leaving"`
	Ready           bool   `json:"ready"`
}

type Payout struct {
	Seat   int   `json:"seat"`
	Amount int64 `json:"amount"`
	Pot    int   `json:"pot"`
}

type SidePot struct {
	Amount   int64 `json:"amount"`
	Eligible []int `json:"eligible"`
}

type Game struct {
	Seats          []*Player
	SmallBlind     int64
	BigBlind       int64
	Button         int
	SmallBlindSeat int
	BigBlindSeat   int
	Actor          int
	Street         Street
	HandNumber     int64
	Board          []Card
	deck           []Card
	deckIndex      int
	CurrentBet     int64
	MinimumRaiseBy int64
	pending        map[int]bool
	actedAtBet     map[int]int64
	LastPayouts    []Payout
	Version        int64
}

func NewGame(maxPlayers int, smallBlind, bigBlind int64) (*Game, error) {
	if maxPlayers < 2 || maxPlayers > 8 {
		return nil, errors.New("max players must be between 2 and 8")
	}
	if smallBlind <= 0 || bigBlind <= smallBlind {
		return nil, errors.New("blinds must be positive and big blind must exceed small blind")
	}
	return &Game{
		Seats: make([]*Player, maxPlayers), SmallBlind: smallBlind, BigBlind: bigBlind,
		Button: -1, SmallBlindSeat: -1, BigBlindSeat: -1, Actor: -1, Street: StreetWaiting,
		pending: make(map[int]bool), actedAtBet: make(map[int]int64),
	}, nil
}

func (g *Game) Sit(userID, name string, seat int, sessionID string) (*Player, error) {
	if seat < 0 || seat >= len(g.Seats) || g.Seats[seat] != nil {
		return nil, ErrSeatOccupied
	}
	for _, player := range g.Seats {
		if player != nil && player.UserID == userID {
			return nil, ErrPlayerSeated
		}
	}
	player := &Player{UserID: userID, Name: name, Seat: seat, Stack: 1000, Allocated: 1000, SeatSessionID: sessionID}
	g.Seats[seat] = player
	g.Version++
	return player, nil
}

func (g *Game) Refill(seat int) error {
	player := g.player(seat)
	if player == nil || g.InHand() || player.Stack != 0 {
		return ErrIllegalAction
	}
	player.Stack += 1000
	player.Allocated += 1000
	player.Away = false
	g.Version++
	return nil
}

func (g *Game) Withdraw(seat int) error {
	player := g.player(seat)
	if player == nil {
		return ErrIllegalAction
	}
	player.Leaving = true
	player.Disconnected = false
	if !g.InHand() {
		player.Away = true
		g.Version++
		return nil
	}
	if player.AllIn {
		g.Version++
		return nil
	}
	player.Folded = true
	if seat == g.Actor {
		g.finishAction(seat, false, false)
		return nil
	}
	delete(g.pending, seat)
	if g.remainingNotFolded() == 1 {
		g.finishByFold()
		return nil
	}
	g.prunePending()
	if len(g.pending) == 0 {
		return g.advanceStreet()
	}
	g.Version++
	return nil
}

func (g *Game) RemoveSeat(seat int) (*Player, error) {
	if g.InHand() {
		return nil, ErrHandInProgress
	}
	player := g.player(seat)
	if player == nil {
		return nil, ErrIllegalAction
	}
	g.Seats[seat] = nil
	g.Version++
	return player, nil
}

func (g *Game) SetDisconnected(seat int, disconnected bool) error {
	player := g.player(seat)
	if player == nil {
		return ErrIllegalAction
	}
	player.Disconnected = disconnected
	g.Version++
	return nil
}

func (g *Game) InHand() bool {
	return g.Street == StreetPreflop || g.Street == StreetFlop || g.Street == StreetTurn || g.Street == StreetRiver || g.Street == StreetShowdown
}

func (g *Game) StartHand() error {
	if g.InHand() {
		return ErrHandInProgress
	}
	if g.countAvailable() < 2 {
		return ErrNotEnoughPlayers
	}
	g.HandNumber++
	g.Board = nil
	g.LastPayouts = nil
	g.pending = make(map[int]bool)
	g.actedAtBet = make(map[int]int64)
	for _, player := range g.Seats {
		if player == nil {
			continue
		}
		player.StreetCommitted = 0
		player.HandCommitted = 0
		player.Hole = nil
		player.Folded = false
		player.AllIn = false
		if player.Stack == 0 {
			player.Away = true
		}
	}
	g.Button = g.nextSeat(g.Button, g.available)
	active := g.activeSeats()
	if len(active) == 2 {
		g.SmallBlindSeat = g.Button
		g.BigBlindSeat = g.nextSeat(g.Button, g.available)
	} else {
		g.SmallBlindSeat = g.nextSeat(g.Button, g.available)
		g.BigBlindSeat = g.nextSeat(g.SmallBlindSeat, g.available)
	}
	g.deck = NewDeck()
	if err := Shuffle(g.deck); err != nil {
		return err
	}
	g.deckIndex = 0
	for round := 0; round < 2; round++ {
		seat := g.Button
		for range len(active) {
			seat = g.nextSeat(seat, g.available)
			if seat < 0 {
				break
			}
			g.Seats[seat].Hole = append(g.Seats[seat].Hole, g.draw())
		}
	}
	g.commit(g.SmallBlindSeat, min(g.SmallBlind, g.Seats[g.SmallBlindSeat].Stack))
	g.commit(g.BigBlindSeat, min(g.BigBlind, g.Seats[g.BigBlindSeat].Stack))
	g.CurrentBet = g.BigBlind
	g.MinimumRaiseBy = g.BigBlind
	g.Street = StreetPreflop
	for _, seat := range active {
		if !g.Seats[seat].AllIn {
			g.pending[seat] = true
		}
	}
	if len(active) == 2 {
		g.Actor = g.firstPendingFrom(g.BigBlindSeat)
	} else {
		g.Actor = g.firstPendingFrom(g.BigBlindSeat)
	}
	g.Version++
	if g.Actor < 0 {
		return g.advanceStreet()
	}
	return nil
}

func (g *Game) ToCall(seat int) int64 {
	player := g.player(seat)
	if player == nil {
		return 0
	}
	return max(0, g.CurrentBet-player.StreetCommitted)
}

func (g *Game) CanRaise(seat int) bool {
	player := g.player(seat)
	if player == nil || seat != g.Actor || player.Folded || player.AllIn || player.Away {
		return false
	}
	toCall := g.ToCall(seat)
	if player.Stack <= toCall {
		return false
	}
	last, acted := g.actedAtBet[seat]
	return !acted || g.CurrentBet-last >= g.MinimumRaiseBy
}

func (g *Game) CheckOrCall(seat int) error {
	player, err := g.requireActor(seat)
	if err != nil {
		return err
	}
	toCall := g.ToCall(seat)
	cost := min(toCall, player.Stack)
	g.commit(seat, cost)
	if player.Stack == 0 {
		player.AllIn = true
	}
	g.finishAction(seat, false, false)
	return nil
}

func (g *Game) Fold(seat int) error {
	player, err := g.requireActor(seat)
	if err != nil {
		return err
	}
	player.Folded = true
	g.finishAction(seat, false, false)
	return nil
}

func (g *Game) Raise(seat int, values, allowed []int64) (chips.RaiseResult, error) {
	player, err := g.requireActor(seat)
	if err != nil {
		return chips.RaiseResult{}, err
	}
	if !g.CanRaise(seat) {
		return chips.RaiseResult{}, ErrIllegalAction
	}
	toCall := g.ToCall(seat)
	result, err := chips.ValidateRaise(values, allowed, toCall, g.MinimumRaiseBy, player.Stack)
	if err != nil {
		return chips.RaiseResult{}, err
	}
	g.commit(seat, result.ActionCost)
	g.CurrentBet += result.RaiseBy
	g.MinimumRaiseBy = result.RaiseBy
	result.RaiseTo = g.CurrentBet
	result.Remaining = player.Stack
	g.finishAction(seat, true, true)
	return result, nil
}

func (g *Game) AllIn(seat int) error {
	player, err := g.requireActor(seat)
	if err != nil {
		return err
	}
	if player.Stack <= 0 {
		return ErrIllegalAction
	}
	newTotal := player.StreetCommitted + player.Stack
	raiseBy := max(0, newTotal-g.CurrentBet)
	fullRaise := raiseBy >= g.MinimumRaiseBy
	g.commit(seat, player.Stack)
	player.AllIn = true
	if raiseBy > 0 {
		g.CurrentBet = newTotal
		if fullRaise {
			g.MinimumRaiseBy = raiseBy
		}
	}
	g.finishAction(seat, raiseBy > 0, fullRaise)
	return nil
}

func (g *Game) Timeout(seat int) error {
	if g.ToCall(seat) == 0 {
		return g.CheckOrCall(seat)
	}
	return g.Fold(seat)
}

func (g *Game) Pot() int64 {
	var total int64
	for _, player := range g.Seats {
		if player != nil {
			total += player.HandCommitted
		}
	}
	return total
}

func (g *Game) SidePots() []SidePot {
	levels := make([]int64, 0, len(g.Seats))
	seen := make(map[int64]bool)
	for _, player := range g.Seats {
		if player != nil && player.HandCommitted > 0 && !seen[player.HandCommitted] {
			seen[player.HandCommitted] = true
			levels = append(levels, player.HandCommitted)
		}
	}
	sort.Slice(levels, func(i, j int) bool { return levels[i] < levels[j] })
	previous := int64(0)
	pots := make([]SidePot, 0, len(levels))
	for _, level := range levels {
		contributors := 0
		eligible := make([]int, 0, len(g.Seats))
		for seat, player := range g.Seats {
			if player != nil && player.HandCommitted >= level {
				contributors++
				if !player.Folded {
					eligible = append(eligible, seat)
				}
			}
		}
		amount := (level - previous) * int64(contributors)
		if amount > 0 {
			pots = append(pots, SidePot{Amount: amount, Eligible: eligible})
		}
		previous = level
	}
	return pots
}

func (g *Game) finishAction(seat int, raised, fullRaise bool) {
	delete(g.pending, seat)
	if fullRaise {
		g.pending = make(map[int]bool)
		for other, player := range g.Seats {
			if other != seat && g.canAct(player) {
				g.pending[other] = true
			}
		}
	} else if raised {
		for other, player := range g.Seats {
			if other != seat && g.canAct(player) && player.StreetCommitted < g.CurrentBet {
				g.pending[other] = true
			}
		}
	}
	g.actedAtBet[seat] = g.CurrentBet
	if g.remainingNotFolded() == 1 {
		g.finishByFold()
		return
	}
	g.prunePending()
	if len(g.pending) == 0 {
		_ = g.advanceStreet()
		return
	}
	g.Actor = g.firstPendingFrom(seat)
	g.Version++
}

func (g *Game) advanceStreet() error {
	if g.Street == StreetRiver {
		return g.showdown()
	}
	if g.Street == StreetPreflop {
		g.burn()
		g.Board = append(g.Board, g.draw(), g.draw(), g.draw())
		g.Street = StreetFlop
	} else if g.Street == StreetFlop {
		g.burn()
		g.Board = append(g.Board, g.draw())
		g.Street = StreetTurn
	} else if g.Street == StreetTurn {
		g.burn()
		g.Board = append(g.Board, g.draw())
		g.Street = StreetRiver
	} else {
		return ErrIllegalAction
	}
	for _, player := range g.Seats {
		if player != nil {
			player.StreetCommitted = 0
		}
	}
	g.CurrentBet = 0
	g.MinimumRaiseBy = g.BigBlind
	g.pending = make(map[int]bool)
	g.actedAtBet = make(map[int]int64)
	for seat, player := range g.Seats {
		if g.canAct(player) {
			g.pending[seat] = true
		}
	}
	if len(g.pending) <= 1 {
		for len(g.Board) < 5 {
			g.burn()
			g.Board = append(g.Board, g.draw())
		}
		return g.showdown()
	}
	g.Actor = g.firstPendingFrom(g.Button)
	g.Version++
	return nil
}

func (g *Game) showdown() error {
	g.Street = StreetShowdown
	for potIndex, pot := range g.SidePots() {
		winners := make([]int, 0, len(pot.Eligible))
		var bestRank uint32
		for _, seat := range pot.Eligible {
			evaluation, err := Evaluate(g.Seats[seat].Hole, g.Board)
			if err != nil {
				return err
			}
			rank := uint32(evaluation.HiRank)
			if len(winners) == 0 || rank < bestRank {
				bestRank = rank
				winners = []int{seat}
			} else if rank == bestRank {
				winners = append(winners, seat)
			}
		}
		if len(winners) == 0 {
			return errors.New("side pot has no eligible winner")
		}
		share := pot.Amount / int64(len(winners))
		remainder := pot.Amount % int64(len(winners))
		ordered := g.orderAfterButton(winners)
		for index, seat := range ordered {
			amount := share
			if int64(index) < remainder {
				amount++
			}
			g.Seats[seat].Stack += amount
			g.LastPayouts = append(g.LastPayouts, Payout{Seat: seat, Amount: amount, Pot: potIndex})
		}
	}
	g.finishHand()
	return nil
}

func (g *Game) finishByFold() {
	winner := -1
	for seat, player := range g.Seats {
		if player != nil && !player.Folded && !player.Away && len(player.Hole) == 2 {
			winner = seat
			break
		}
	}
	if winner >= 0 {
		amount := g.Pot()
		g.Seats[winner].Stack += amount
		g.LastPayouts = []Payout{{Seat: winner, Amount: amount, Pot: 0}}
	}
	g.finishHand()
}

func (g *Game) finishHand() {
	g.Street = StreetSettled
	g.Actor = -1
	g.pending = make(map[int]bool)
	for _, player := range g.Seats {
		if player != nil && player.Stack == 0 {
			player.Away = true
		}
	}
	g.Version++
}

func (g *Game) commit(seat int, amount int64) {
	player := g.Seats[seat]
	amount = min(amount, player.Stack)
	player.Stack -= amount
	player.StreetCommitted += amount
	player.HandCommitted += amount
	if player.Stack == 0 {
		player.AllIn = true
	}
}

func (g *Game) draw() Card {
	card := g.deck[g.deckIndex]
	g.deckIndex++
	return card
}

func (g *Game) burn() {
	if g.deckIndex < len(g.deck) {
		g.deckIndex++
	}
}

func (g *Game) player(seat int) *Player {
	if seat < 0 || seat >= len(g.Seats) {
		return nil
	}
	return g.Seats[seat]
}

func (g *Game) requireActor(seat int) (*Player, error) {
	if seat != g.Actor {
		return nil, ErrNotPlayersTurn
	}
	player := g.player(seat)
	if !g.canAct(player) {
		return nil, ErrIllegalAction
	}
	return player, nil
}

func (g *Game) available(player *Player) bool {
	return player != nil && player.Ready && !player.Away && !player.Disconnected && !player.Leaving && player.Stack > 0
}

func (g *Game) canAct(player *Player) bool {
	return player != nil && !player.Away && !player.Leaving && !player.Folded && !player.AllIn && player.Stack > 0
}

func (g *Game) activeSeats() []int {
	result := make([]int, 0, len(g.Seats))
	for seat, player := range g.Seats {
		if g.available(player) {
			result = append(result, seat)
		}
	}
	return result
}

func (g *Game) countAvailable() int {
	return len(g.activeSeats())
}

func (g *Game) remainingNotFolded() int {
	count := 0
	for _, player := range g.Seats {
		if player != nil && !player.Folded && !player.Away && (player.HandCommitted > 0 || len(player.Hole) == 2) {
			count++
		}
	}
	return count
}

func (g *Game) nextSeat(start int, predicate func(*Player) bool) int {
	for offset := 1; offset <= len(g.Seats); offset++ {
		seat := (start + offset + len(g.Seats)) % len(g.Seats)
		if predicate(g.Seats[seat]) {
			return seat
		}
	}
	return -1
}

func (g *Game) firstPendingFrom(start int) int {
	return g.nextSeat(start, func(player *Player) bool { return player != nil && g.pending[player.Seat] })
}

func (g *Game) prunePending() {
	for seat := range g.pending {
		if !g.canAct(g.Seats[seat]) {
			delete(g.pending, seat)
		}
	}
}

func (g *Game) orderAfterButton(seats []int) []int {
	selected := make(map[int]bool, len(seats))
	for _, seat := range seats {
		selected[seat] = true
	}
	ordered := make([]int, 0, len(seats))
	for offset := 1; offset <= len(g.Seats); offset++ {
		seat := (g.Button + offset) % len(g.Seats)
		if selected[seat] {
			ordered = append(ordered, seat)
		}
	}
	return ordered
}

func (g *Game) DebugState() string {
	return fmt.Sprintf("hand=%d street=%s actor=%d bet=%d minRaise=%d pot=%d", g.HandNumber, g.Street, g.Actor, g.CurrentBet, g.MinimumRaiseBy, g.Pot())
}

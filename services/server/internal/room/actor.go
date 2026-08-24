package room

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
	"github.com/royal-flush/royal-flush/services/server/internal/poker"
	"github.com/royal-flush/royal-flush/services/server/internal/score"
)

var (
	ErrRoomClosed          = errors.New("room is closed")
	ErrForbidden           = errors.New("only the room owner can perform this operation")
	ErrCannotRemoveOwner   = errors.New("the room owner cannot remove themselves")
	ErrVersionConflict     = errors.New("room version does not match expectedVersion")
	ErrPlayerNotSeated     = errors.New("player is not seated in this room")
	ErrPlayersNotReady     = errors.New("at least two ready players are required")
	ErrInvalidQuickMessage = errors.New("quick message is not supported")
)

type Identity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AccountScores interface {
	Balance(userID string) int64
	ApplySettlement(userID, roomID, seatSessionID string, net int64) (score.Result, error)
}

type PlayerSnapshot struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Initials        string `json:"initials"`
	Seat            int    `json:"seat"`
	TablePoints     int64  `json:"tablePoints"`
	AccountPoints   int64  `json:"accountPoints"`
	StreetCommitted int64  `json:"streetCommitted"`
	Status          string `json:"status"`
	IsDealer        bool   `json:"isDealer"`
	IsSpeaking      bool   `json:"isSpeaking"`
	IsCurrentActor  bool   `json:"isCurrentActor"`
	IsLocal         bool   `json:"isLocal"`
	IsReady         bool   `json:"isReady"`
	IsMuted         bool   `json:"isMuted"`
}

type TableSnapshot struct {
	RoomID                   string           `json:"roomId"`
	RoomCode                 string           `json:"roomCode"`
	RoomName                 string           `json:"roomName"`
	OwnerID                  string           `json:"ownerId"`
	Version                  int64            `json:"version"`
	HandNumber               int64            `json:"handNumber"`
	Street                   poker.Street     `json:"street"`
	Pot                      int64            `json:"pot"`
	Board                    []poker.Card     `json:"board"`
	HoleCards                []poker.Card     `json:"holeCards"`
	Players                  []PlayerSnapshot `json:"players"`
	AllowedChipDenominations []int64          `json:"allowedChipDenominations"`
	ToCall                   int64            `json:"toCall"`
	MinimumRaiseBy           int64            `json:"minimumRaiseBy"`
	MaximumRaiseBy           int64            `json:"maximumRaiseBy"`
	CanCheck                 bool             `json:"canCheck"`
	CanCall                  bool             `json:"canCall"`
	CanRaise                 bool             `json:"canRaise"`
	CanAllIn                 bool             `json:"canAllIn"`
	ActionDeadline           time.Time        `json:"actionDeadline"`
	Config                   Config           `json:"config"`
	Messages                 []SystemMessage  `json:"messages"`
	Ended                    bool             `json:"ended"`
}

type SystemMessage struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type Envelope struct {
	Type      string    `json:"type"`
	RequestID string    `json:"requestId"`
	RoomID    string    `json:"roomId"`
	Version   int64     `json:"version"`
	SentAt    time.Time `json:"sentAt"`
	Payload   any       `json:"payload"`
}

type ClientCommand struct {
	Type            string          `json:"type"`
	RequestID       string          `json:"requestId"`
	ExpectedVersion int64           `json:"expectedVersion"`
	Payload         json.RawMessage `json:"payload"`
}

type commandResult struct {
	Envelope  Envelope
	Duplicate bool
}

type actorCall struct {
	fn       func() (any, error)
	response chan actorResponse
}

type actorResponse struct {
	value any
	err   error
}

type timeoutSignal struct {
	version int64
	seat    int
}

type disconnectSignal struct {
	userID     string
	generation int64
}

type Actor struct {
	ID        string
	Code      string
	OwnerID   string
	CreatedAt time.Time

	config         Config
	game           *poker.Game
	scores         AccountScores
	identities     map[string]Identity
	joinOrder      map[string]int64
	nextJoin       int64
	muted          map[string]bool
	messages       []SystemMessage
	version        int64
	deadline       time.Time
	ended          bool
	processed      map[string]Envelope
	subscribers    map[chan Envelope]struct{}
	connections    map[string]int
	disconnectGen  map[string]int64
	disconnectWait time.Duration
	calls          chan actorCall
	timeouts       chan timeoutSignal
	disconnects    chan disconnectSignal
	stop           chan struct{}
	onSeatClosed   func(userID string)
	onCodeChanged  func(oldCode, newCode string)
}

func NewActor(config Config, owner Identity, scores AccountScores, onSeatClosed func(string)) (*Actor, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if owner.ID == "" || owner.Name == "" {
		return nil, errors.New("owner identity is required")
	}
	id, err := idgen.ID("room")
	if err != nil {
		return nil, err
	}
	code, err := idgen.RoomCode()
	if err != nil {
		return nil, err
	}
	smallBlind, bigBlind, _ := config.Blinds()
	game, err := poker.NewGame(config.MaxPlayers, smallBlind, bigBlind)
	if err != nil {
		return nil, err
	}
	sessionID, err := idgen.ID("seat")
	if err != nil {
		return nil, err
	}
	if _, err := game.Sit(owner.ID, owner.Name, 0, sessionID); err != nil {
		return nil, err
	}
	actor := &Actor{
		ID: id, Code: code, OwnerID: owner.ID, CreatedAt: time.Now().UTC(), config: cloneConfig(config),
		game: game, scores: scores, identities: map[string]Identity{owner.ID: owner}, joinOrder: map[string]int64{owner.ID: 1}, nextJoin: 1, muted: make(map[string]bool),
		processed: make(map[string]Envelope), subscribers: make(map[chan Envelope]struct{}), connections: make(map[string]int), disconnectGen: make(map[string]int64), disconnectWait: 60 * time.Second, calls: make(chan actorCall),
		timeouts: make(chan timeoutSignal, 8), disconnects: make(chan disconnectSignal, 8), stop: make(chan struct{}), onSeatClosed: onSeatClosed, version: 1,
	}
	actor.appendMessage("system", fmt.Sprintf("%s 创建了房间", owner.Name))
	go actor.loop()
	return actor, nil
}

func (a *Actor) Close() {
	select {
	case <-a.stop:
		return
	default:
		close(a.stop)
	}
}

func (a *Actor) Join(ctx context.Context, identity Identity, seat int) (TableSnapshot, error) {
	value, err := a.call(ctx, func() (any, error) {
		if a.ended {
			return nil, ErrRoomClosed
		}
		sessionID, err := idgen.ID("seat")
		if err != nil {
			return nil, err
		}
		if _, err := a.game.Sit(identity.ID, identity.Name, seat, sessionID); err != nil {
			return nil, err
		}
		a.identities[identity.ID] = identity
		a.nextJoin++
		a.joinOrder[identity.ID] = a.nextJoin
		if a.OwnerID == "" {
			a.OwnerID = identity.ID
		}
		a.version++
		a.appendMessage("room", fmt.Sprintf("%s 坐入 %d 号位", identity.Name, seat+1))
		a.publish("room.player_joined", "", map[string]any{"userId": identity.ID, "seat": seat})
		return a.snapshot(identity.ID), nil
	})
	if err != nil {
		return TableSnapshot{}, err
	}
	return value.(TableSnapshot), nil
}

func (a *Actor) Snapshot(ctx context.Context, userID string) (TableSnapshot, error) {
	value, err := a.call(ctx, func() (any, error) {
		if a.seatFor(userID) < 0 {
			return nil, ErrPlayerNotSeated
		}
		return a.snapshot(userID), nil
	})
	if err != nil {
		return TableSnapshot{}, err
	}
	return value.(TableSnapshot), nil
}

func (a *Actor) PublicSnapshot(ctx context.Context) (TableSnapshot, error) {
	value, err := a.call(ctx, func() (any, error) {
		return a.snapshot(""), nil
	})
	if err != nil {
		return TableSnapshot{}, err
	}
	return value.(TableSnapshot), nil
}

func (a *Actor) Handle(ctx context.Context, userID string, command ClientCommand) (Envelope, bool, error) {
	value, err := a.call(ctx, func() (any, error) {
		if a.ended {
			return nil, ErrRoomClosed
		}
		if command.RequestID == "" {
			return nil, score.ErrRequestID
		}
		key := userID + "\x00" + command.RequestID
		if existing, ok := a.processed[key]; ok {
			return commandResult{Envelope: existing, Duplicate: true}, nil
		}
		seat := a.seatFor(userID)
		if seat < 0 {
			return nil, ErrPlayerNotSeated
		}
		if strings.HasPrefix(command.Type, "action.") && command.ExpectedVersion != a.version {
			return nil, ErrVersionConflict
		}
		payload, eventType, err := a.applyCommand(userID, seat, command)
		if err != nil {
			return nil, err
		}
		a.version++
		envelope := a.makeEnvelope(eventType, command.RequestID, payload)
		a.processed[key] = envelope
		a.publishEnvelope(envelope)
		if a.game.InHand() && a.game.Actor >= 0 {
			a.scheduleActionTimeout()
		}
		if !a.game.InHand() {
			a.settleLeaving()
		}
		return commandResult{Envelope: envelope}, nil
	})
	if err != nil {
		return Envelope{}, false, err
	}
	result := value.(commandResult)
	return result.Envelope, result.Duplicate, nil
}

func (a *Actor) Subscribe(ctx context.Context) (<-chan Envelope, func(), error) {
	value, err := a.call(ctx, func() (any, error) {
		channel := make(chan Envelope, 64)
		a.subscribers[channel] = struct{}{}
		return channel, nil
	})
	if err != nil {
		return nil, nil, err
	}
	channel := value.(chan Envelope)
	cancel := func() {
		_, _ = a.call(context.Background(), func() (any, error) {
			if _, ok := a.subscribers[channel]; ok {
				delete(a.subscribers, channel)
				close(channel)
			}
			return nil, nil
		})
	}
	return channel, cancel, nil
}

func (a *Actor) PlayerConnected(ctx context.Context, userID string) error {
	_, err := a.call(ctx, func() (any, error) {
		seat := a.seatFor(userID)
		if seat < 0 {
			return nil, ErrPlayerNotSeated
		}
		a.connections[userID]++
		a.disconnectGen[userID]++
		player := a.game.Seats[seat]
		if player.Disconnected {
			if err := a.game.SetDisconnected(seat, false); err != nil {
				return nil, err
			}
			a.version++
			a.publish("room.player_reconnected", "", map[string]any{"userId": userID})
		}
		return nil, nil
	})
	return err
}

func (a *Actor) PlayerDisconnected(ctx context.Context, userID string) error {
	_, err := a.call(ctx, func() (any, error) {
		seat := a.seatFor(userID)
		if seat < 0 {
			return nil, ErrPlayerNotSeated
		}
		if a.connections[userID] > 0 {
			a.connections[userID]--
		}
		if a.connections[userID] > 0 || a.game.Seats[seat].Disconnected {
			return nil, nil
		}
		if err := a.game.SetDisconnected(seat, true); err != nil {
			return nil, err
		}
		a.version++
		a.publish("room.player_disconnected", "", map[string]any{"userId": userID, "retainedSeconds": int(a.disconnectWait.Seconds())})
		a.disconnectGen[userID]++
		signal := disconnectSignal{userID: userID, generation: a.disconnectGen[userID]}
		time.AfterFunc(a.disconnectWait, func() {
			select {
			case a.disconnects <- signal:
			case <-a.stop:
			}
		})
		return nil, nil
	})
	return err
}

func (a *Actor) BroadcastScoreAddition(ctx context.Context, userID, requestID string, amount, balance int64) error {
	_, err := a.call(ctx, func() (any, error) {
		key := "score-addition\x00" + userID + "\x00" + requestID
		if _, ok := a.processed[key]; ok {
			return nil, nil
		}
		identity := a.identities[userID]
		if identity.ID == "" {
			return nil, ErrPlayerNotSeated
		}
		text := fmt.Sprintf("%s 自行增加了 %d 积分，当前局外积分为 %d", identity.Name, amount, balance)
		message := a.appendMessage("score", text)
		a.version++
		envelope := a.makeEnvelope("score.self_added", requestID, map[string]any{"userId": userID, "amount": amount, "balance": balance, "message": message})
		a.processed[key] = envelope
		a.publishEnvelope(envelope)
		return nil, nil
	})
	return err
}

func (a *Actor) BroadcastGlobalReset(ctx context.Context, epoch score.Epoch, requestID string) error {
	_, err := a.call(ctx, func() (any, error) {
		message := a.appendMessage("score", "平台管理员已将所有账号的局外积分重置为 1,000，本局继续，结束后照常结算净输赢。")
		a.version++
		a.publish("score.global_reset", requestID, map[string]any{"epoch": epoch, "message": message})
		return nil, nil
	})
	return err
}

func (a *Actor) applyCommand(userID string, seat int, command ClientCommand) (any, string, error) {
	player := a.game.Seats[seat]
	switch command.Type {
	case "room.ready":
		var payload struct {
			Ready bool `json:"ready"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, "", err
		}
		player.Ready = payload.Ready
		return map[string]any{"userId": userID, "ready": payload.Ready}, "room.ready_changed", nil
	case "game.start":
		if userID != a.OwnerID {
			return nil, "", ErrForbidden
		}
		ready := 0
		for _, participant := range a.game.Seats {
			if participant != nil && participant.Ready && !participant.Leaving {
				ready++
			}
		}
		if ready < 2 {
			return nil, "", ErrPlayersNotReady
		}
		if err := a.game.StartHand(); err != nil {
			return nil, "", err
		}
		return map[string]any{"handNumber": a.game.HandNumber}, "game.hand_started", nil
	case "action.raise":
		var payload struct {
			Chips []int64 `json:"chips"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, "", err
		}
		result, err := a.game.Raise(seat, payload.Chips, a.config.ChipDenominations)
		if err != nil {
			return nil, "", err
		}
		return result, "game.action_applied", nil
	case "action.call", "action.check":
		if command.Type == "action.check" && a.game.ToCall(seat) != 0 {
			return nil, "", poker.ErrIllegalAction
		}
		toCall := a.game.ToCall(seat)
		if err := a.game.CheckOrCall(seat); err != nil {
			return nil, "", err
		}
		return map[string]any{"action": strings.TrimPrefix(command.Type, "action."), "amount": toCall}, "game.action_applied", nil
	case "action.fold":
		if err := a.game.Fold(seat); err != nil {
			return nil, "", err
		}
		return map[string]any{"action": "fold"}, "game.action_applied", nil
	case "action.all_in":
		amount := player.Stack
		if err := a.game.AllIn(seat); err != nil {
			return nil, "", err
		}
		return map[string]any{"action": "all_in", "amount": amount}, "game.action_applied", nil
	case "room.refill":
		if err := a.game.Refill(seat); err != nil {
			return nil, "", err
		}
		return map[string]any{"tablePoints": player.Stack, "allocatedTablePoints": player.Allocated}, "room.table_points_refilled", nil
	case "room.leave":
		if err := a.game.Withdraw(seat); err != nil {
			return nil, "", err
		}
		return map[string]any{"userId": userID}, "room.player_leaving", nil
	case "room.quick_message":
		var payload struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, "", err
		}
		allowed := map[string]bool{"好牌": true, "稍等一下": true, "打得漂亮": true, "下一手": true}
		if !allowed[payload.Message] {
			return nil, "", ErrInvalidQuickMessage
		}
		message := a.appendMessage("quick", fmt.Sprintf("%s：%s", player.Name, payload.Message))
		return message, "room.quick_message", nil
	case "voice.mute":
		if userID != a.OwnerID {
			return nil, "", ErrForbidden
		}
		var payload struct {
			UserID string `json:"userId"`
			Muted  bool   `json:"muted"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, "", err
		}
		if a.seatFor(payload.UserID) < 0 {
			return nil, "", ErrPlayerNotSeated
		}
		a.muted[payload.UserID] = payload.Muted
		return payload, "voice.mute_changed", nil
	case "room.remove_player":
		if userID != a.OwnerID {
			return nil, "", ErrForbidden
		}
		var payload struct {
			UserID string `json:"userId"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, "", err
		}
		if payload.UserID == a.OwnerID {
			return nil, "", ErrCannotRemoveOwner
		}
		targetSeat := a.seatFor(payload.UserID)
		if targetSeat < 0 {
			return nil, "", ErrPlayerNotSeated
		}
		if err := a.game.Withdraw(targetSeat); err != nil {
			return nil, "", err
		}
		a.appendMessage("room", fmt.Sprintf("%s 被房主移出房间", a.identities[payload.UserID].Name))
		return payload, "room.player_removed", nil
	case "room.rotate_invite":
		if userID != a.OwnerID {
			return nil, "", ErrForbidden
		}
		code, err := idgen.RoomCode()
		if err != nil {
			return nil, "", err
		}
		oldCode := a.Code
		a.Code = code
		if a.onCodeChanged != nil {
			a.onCodeChanged(oldCode, code)
		}
		return map[string]any{"roomCode": code}, "room.invite_rotated", nil
	case "room.transfer_owner":
		if userID != a.OwnerID {
			return nil, "", ErrForbidden
		}
		var payload struct {
			UserID string `json:"userId"`
		}
		if err := json.Unmarshal(command.Payload, &payload); err != nil {
			return nil, "", err
		}
		if a.seatFor(payload.UserID) < 0 {
			return nil, "", ErrPlayerNotSeated
		}
		a.OwnerID = payload.UserID
		return payload, "room.owner_transferred", nil
	case "room.end":
		if userID != a.OwnerID {
			return nil, "", ErrForbidden
		}
		if a.game.InHand() {
			return nil, "", poker.ErrHandInProgress
		}
		a.ended = true
		a.settleAll()
		return map[string]any{"ended": true}, "room.ended", nil
	default:
		return nil, "", fmt.Errorf("unsupported command %q", command.Type)
	}
}

func (a *Actor) snapshot(userID string) TableSnapshot {
	players := make([]PlayerSnapshot, 0, len(a.game.Seats))
	localSeat := a.seatFor(userID)
	for seat, player := range a.game.Seats {
		if player == nil {
			continue
		}
		status := "active"
		switch {
		case player.Disconnected:
			status = "disconnected"
		case player.Leaving || player.Away:
			status = "away"
		case player.Folded:
			status = "folded"
		case player.AllIn:
			status = "all-in"
		}
		players = append(players, PlayerSnapshot{
			ID: player.UserID, Name: player.Name, Initials: firstRune(player.Name), Seat: seat,
			TablePoints: player.Stack, AccountPoints: a.scores.Balance(player.UserID), StreetCommitted: player.StreetCommitted,
			Status: status, IsDealer: seat == a.game.Button, IsCurrentActor: seat == a.game.Actor,
			IsLocal: player.UserID == userID, IsReady: player.Ready, IsMuted: a.muted[player.UserID],
		})
	}
	toCall, maximumRaise, canCheck, canCall, canRaise, canAllIn := int64(0), int64(0), false, false, false, false
	hole := []poker.Card(nil)
	if localSeat >= 0 {
		player := a.game.Seats[localSeat]
		hole = append(hole, player.Hole...)
		if localSeat == a.game.Actor && a.game.InHand() {
			toCall = a.game.ToCall(localSeat)
			maximumRaise = max(0, player.Stack-toCall)
			canCheck = toCall == 0
			canCall = true
			canRaise = a.game.CanRaise(localSeat)
			canAllIn = player.Stack > 0
		}
	}
	return TableSnapshot{
		RoomID: a.ID, RoomCode: a.Code, RoomName: a.config.Name, OwnerID: a.OwnerID, Version: a.version,
		HandNumber: a.game.HandNumber, Street: a.game.Street, Pot: a.game.Pot(), Board: append([]poker.Card(nil), a.game.Board...),
		HoleCards: hole, Players: players, AllowedChipDenominations: append([]int64(nil), a.config.ChipDenominations...),
		ToCall: toCall, MinimumRaiseBy: a.game.MinimumRaiseBy, MaximumRaiseBy: maximumRaise,
		CanCheck: canCheck, CanCall: canCall, CanRaise: canRaise, CanAllIn: canAllIn,
		ActionDeadline: a.deadline, Config: cloneConfig(a.config), Messages: append([]SystemMessage(nil), a.messages...), Ended: a.ended,
	}
}

func (a *Actor) loop() {
	for {
		select {
		case call := <-a.calls:
			value, err := call.fn()
			call.response <- actorResponse{value: value, err: err}
		case signal := <-a.timeouts:
			a.handleTimeout(signal)
		case signal := <-a.disconnects:
			a.handleDisconnectTimeout(signal)
		case <-a.stop:
			for channel := range a.subscribers {
				close(channel)
			}
			return
		}
	}
}

func (a *Actor) call(ctx context.Context, fn func() (any, error)) (any, error) {
	response := make(chan actorResponse, 1)
	select {
	case a.calls <- actorCall{fn: fn, response: response}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.stop:
		return nil, ErrRoomClosed
	}
	select {
	case result := <-response:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-a.stop:
		return nil, ErrRoomClosed
	}
}

func (a *Actor) scheduleActionTimeout() {
	a.deadline = time.Now().UTC().Add(time.Duration(a.config.ActionSeconds) * time.Second)
	signal := timeoutSignal{version: a.version, seat: a.game.Actor}
	time.AfterFunc(time.Duration(a.config.ActionSeconds)*time.Second, func() {
		select {
		case a.timeouts <- signal:
		case <-a.stop:
		}
	})
}

func (a *Actor) handleTimeout(signal timeoutSignal) {
	if a.ended || !a.game.InHand() || signal.version != a.version || signal.seat != a.game.Actor {
		return
	}
	toCall := a.game.ToCall(signal.seat)
	if err := a.game.Timeout(signal.seat); err != nil {
		return
	}
	a.version++
	a.publish("game.action_timed_out", "", map[string]any{"seat": signal.seat, "action": map[bool]string{true: "check", false: "fold"}[toCall == 0]})
	if a.game.InHand() && a.game.Actor >= 0 {
		a.scheduleActionTimeout()
	} else {
		a.settleLeaving()
	}
}

func (a *Actor) handleDisconnectTimeout(signal disconnectSignal) {
	if a.ended || a.disconnectGen[signal.userID] != signal.generation || a.connections[signal.userID] > 0 {
		return
	}
	seat := a.seatFor(signal.userID)
	if seat < 0 || !a.game.Seats[seat].Disconnected {
		return
	}
	if err := a.game.Withdraw(seat); err != nil {
		return
	}
	a.version++
	a.publish("room.player_leaving", "", map[string]any{"userId": signal.userID, "reason": "disconnect_timeout"})
	if a.game.InHand() && a.game.Actor >= 0 {
		a.scheduleActionTimeout()
	}
	if !a.game.InHand() {
		a.settleLeaving()
	}
}

func (a *Actor) seatFor(userID string) int {
	for seat, player := range a.game.Seats {
		if player != nil && player.UserID == userID {
			return seat
		}
	}
	return -1
}

func (a *Actor) settleLeaving() {
	for seat, player := range a.game.Seats {
		if player != nil && player.Leaving {
			a.settleSeat(seat)
		}
	}
}

func (a *Actor) settleAll() {
	for seat, player := range a.game.Seats {
		if player != nil {
			a.settleSeat(seat)
		}
	}
}

func (a *Actor) settleSeat(seat int) {
	player, err := a.game.RemoveSeat(seat)
	if err != nil {
		return
	}
	net := player.Stack - player.Allocated
	result, err := a.scores.ApplySettlement(player.UserID, a.ID, player.SeatSessionID, net)
	if err == nil {
		a.publish("score.settlement_applied", player.SeatSessionID, map[string]any{"userId": player.UserID, "net": net, "balance": result.Balance})
	}
	delete(a.identities, player.UserID)
	delete(a.joinOrder, player.UserID)
	delete(a.muted, player.UserID)
	delete(a.connections, player.UserID)
	delete(a.disconnectGen, player.UserID)
	if a.onSeatClosed != nil {
		a.onSeatClosed(player.UserID)
	}
	if player.UserID == a.OwnerID {
		a.transferOwnerAfterDeparture()
	}
}

func (a *Actor) transferOwnerAfterDeparture() {
	var candidate *poker.Player
	var candidateOrder int64
	for _, player := range a.game.Seats {
		if player == nil || player.Disconnected || player.Leaving {
			continue
		}
		order := a.joinOrder[player.UserID]
		if candidate == nil || order < candidateOrder {
			candidate = player
			candidateOrder = order
		}
	}
	if candidate != nil {
		a.OwnerID = candidate.UserID
		a.publish("room.owner_transferred", "", map[string]any{"userId": candidate.UserID, "automatic": true})
		return
	}
	a.OwnerID = ""
}

func (a *Actor) appendMessage(kind, text string) SystemMessage {
	id, _ := idgen.ID("message")
	message := SystemMessage{ID: id, Type: kind, Text: text, CreatedAt: time.Now().UTC()}
	a.messages = append([]SystemMessage{message}, a.messages...)
	if len(a.messages) > 100 {
		a.messages = a.messages[:100]
	}
	return message
}

func (a *Actor) makeEnvelope(kind, requestID string, payload any) Envelope {
	return Envelope{Type: kind, RequestID: requestID, RoomID: a.ID, Version: a.version, SentAt: time.Now().UTC(), Payload: payload}
}

func (a *Actor) publish(kind, requestID string, payload any) {
	a.publishEnvelope(a.makeEnvelope(kind, requestID, payload))
}

func (a *Actor) publishEnvelope(envelope Envelope) {
	for channel := range a.subscribers {
		select {
		case channel <- envelope:
		default:
		}
	}
}

func cloneConfig(config Config) Config {
	result := config
	result.ChipDenominations = append([]int64(nil), config.ChipDenominations...)
	return result
}

func firstRune(value string) string {
	if value == "" {
		return "?"
	}
	runeValue, _ := utf8.DecodeRuneInString(value)
	return string(runeValue)
}

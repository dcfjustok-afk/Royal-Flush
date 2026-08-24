package loadtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

type Config struct {
	Target            string
	Connections       int
	Rooms             int
	Actions           int
	Reconnects        int
	SetupConcurrency  int
	SocketConcurrency int
	RequestTimeout    time.Duration
	ActionP95Limit    time.Duration
	ReconnectMaxLimit time.Duration
}

func (c Config) Validate() error {
	parsed, err := url.Parse(c.Target)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("target must be an absolute http or https URL")
	}
	if c.Rooms < 1 {
		return errors.New("rooms must be at least 1")
	}
	if c.Connections < c.Rooms*2 {
		return errors.New("connections must provide at least two seated players per room")
	}
	if c.Actions < 1 || c.Actions > c.Rooms {
		return errors.New("actions must be between 1 and the room count")
	}
	if c.Reconnects < 1 || c.Reconnects > c.Rooms {
		return errors.New("reconnects must be between 1 and the room count")
	}
	if c.SetupConcurrency < 1 || c.SocketConcurrency < 1 {
		return errors.New("setup and socket concurrency must be at least 1")
	}
	if c.RequestTimeout <= 0 || c.ActionP95Limit <= 0 || c.ReconnectMaxLimit <= 0 {
		return errors.New("timeouts and latency limits must be positive")
	}
	return nil
}

type Workload struct {
	Connections int `json:"connections"`
	Rooms       int `json:"rooms"`
	Actions     int `json:"actions"`
	Reconnects  int `json:"reconnects"`
}

type Observed struct {
	EstablishedConnections int `json:"establishedConnections"`
	ActiveRooms            int `json:"activeRooms"`
}

type LatencyMetrics struct {
	Samples     int     `json:"samples"`
	MinimumMs   float64 `json:"minimumMs"`
	P50Ms       float64 `json:"p50Ms"`
	P95Ms       float64 `json:"p95Ms"`
	MaximumMs   float64 `json:"maximumMs"`
	ThresholdMs float64 `json:"thresholdMs"`
	Evaluation  string  `json:"evaluation"`
	Passed      bool    `json:"passed"`
}

type PhaseDurations struct {
	RoomSetupMs   int64 `json:"roomSetupMs"`
	SocketSetupMs int64 `json:"socketSetupMs"`
	ActionMs      int64 `json:"actionMs"`
	ReconnectMs   int64 `json:"reconnectMs"`
}

type Report struct {
	RunID         string         `json:"runId"`
	Target        string         `json:"target"`
	StartedAt     time.Time      `json:"startedAt"`
	DurationMs    int64          `json:"durationMs"`
	Requested     Workload       `json:"requested"`
	Observed      Observed       `json:"observed"`
	Phases        PhaseDurations `json:"phases"`
	ActionLatency LatencyMetrics `json:"actionLatency"`
	Reconnect     LatencyMetrics `json:"reconnectSnapshotLatency"`
	Passed        bool           `json:"passed"`
	Failures      []string       `json:"failures,omitempty"`
}

type runner struct {
	runID      string
	config     Config
	httpClient *http.Client
	rooms      []*testRoom
	sockets    []*socketClient
}

type testRoom struct {
	id           string
	owner        *participant
	participants []*participant
	byID         map[string]*participant
}

type participant struct {
	id      string
	name    string
	sockets []*socketClient
	primary *socketClient
}

type tableSnapshot struct {
	RoomID  string           `json:"roomId"`
	Version int64            `json:"version"`
	Players []playerSnapshot `json:"players"`
}

type playerSnapshot struct {
	ID             string `json:"id"`
	Status         string `json:"status"`
	IsCurrentActor bool   `json:"isCurrentActor"`
}

type envelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	RoomID    string          `json:"roomId"`
	Version   int64           `json:"version"`
	Payload   json.RawMessage `json:"payload"`
}

type clientCommand struct {
	Type            string `json:"type"`
	RequestID       string `json:"requestId"`
	ExpectedVersion int64  `json:"expectedVersion"`
	Payload         any    `json:"payload"`
}

type socketResult struct {
	event envelope
	err   error
}

type socketClient struct {
	connection *websocket.Conn
	cancel     context.CancelFunc
	writeMu    sync.Mutex
	mu         sync.Mutex
	pending    map[string]chan socketResult
	closed     bool
}

func Run(ctx context.Context, config Config) (report Report, runErr error) {
	runID := fmt.Sprintf("load-%x", time.Now().UnixNano())
	report = Report{
		RunID: runID, Target: config.Target, StartedAt: time.Now().UTC(),
		Requested: Workload{Connections: config.Connections, Rooms: config.Rooms, Actions: config.Actions, Reconnects: config.Reconnects},
	}
	started := time.Now()
	defer func() { report.DurationMs = time.Since(started).Milliseconds() }()
	if err := config.Validate(); err != nil {
		report.Failures = append(report.Failures, err.Error())
		return report, err
	}
	r := &runner{
		runID: runID, config: config,
		httpClient: &http.Client{
			Timeout: config.RequestTimeout,
			Transport: &http.Transport{
				MaxIdleConns:        config.SocketConcurrency * 2,
				MaxIdleConnsPerHost: config.SocketConcurrency * 2,
				IdleConnTimeout:     30 * time.Second,
			},
		},
	}
	defer r.close()

	phaseStarted := time.Now()
	if err := r.setupRooms(ctx); err != nil {
		report.Failures = append(report.Failures, "room setup: "+err.Error())
		return report, err
	}
	report.Phases.RoomSetupMs = time.Since(phaseStarted).Milliseconds()
	report.Observed.ActiveRooms = len(r.rooms)

	phaseStarted = time.Now()
	if err := r.openSockets(ctx); err != nil {
		report.Failures = append(report.Failures, "websocket setup: "+err.Error())
		return report, err
	}
	report.Phases.SocketSetupMs = time.Since(phaseStarted).Milliseconds()
	report.Observed.EstablishedConnections = len(r.sockets)

	if err := r.startGames(ctx); err != nil {
		report.Failures = append(report.Failures, "game start: "+err.Error())
		return report, err
	}
	phaseStarted = time.Now()
	actionDurations, err := r.measureActions(ctx)
	report.Phases.ActionMs = time.Since(phaseStarted).Milliseconds()
	if err != nil {
		report.Failures = append(report.Failures, "action measurement: "+err.Error())
		return report, err
	}
	report.ActionLatency = summarize(actionDurations, config.ActionP95Limit, "p95")

	phaseStarted = time.Now()
	reconnectDurations, err := r.measureReconnects(ctx)
	report.Phases.ReconnectMs = time.Since(phaseStarted).Milliseconds()
	if err != nil {
		report.Failures = append(report.Failures, "reconnect measurement: "+err.Error())
		return report, err
	}
	report.Reconnect = summarize(reconnectDurations, config.ReconnectMaxLimit, "maximum")

	if report.Observed.ActiveRooms != config.Rooms {
		report.Failures = append(report.Failures, fmt.Sprintf("active rooms = %d, expected %d", report.Observed.ActiveRooms, config.Rooms))
	}
	if report.Observed.EstablishedConnections != config.Connections {
		report.Failures = append(report.Failures, fmt.Sprintf("established connections = %d, expected %d", report.Observed.EstablishedConnections, config.Connections))
	}
	if !report.ActionLatency.Passed {
		report.Failures = append(report.Failures, fmt.Sprintf("action p95 %.3fms exceeded %.3fms", report.ActionLatency.P95Ms, report.ActionLatency.ThresholdMs))
	}
	if !report.Reconnect.Passed {
		report.Failures = append(report.Failures, fmt.Sprintf("reconnect maximum %.3fms exceeded %.3fms", report.Reconnect.MaximumMs, report.Reconnect.ThresholdMs))
	}
	report.Passed = len(report.Failures) == 0
	return report, nil
}

func (r *runner) setupRooms(ctx context.Context) error {
	uniqueCounts := uniqueParticipants(r.config.Connections, r.config.Rooms)
	r.rooms = make([]*testRoom, r.config.Rooms)
	return parallel(ctx, r.config.Rooms, r.config.SetupConcurrency, func(ctx context.Context, roomIndex int) error {
		participants := make([]*participant, uniqueCounts[roomIndex])
		for seat := range participants {
			participants[seat] = &participant{
				id:   fmt.Sprintf("%s-r%03d-u%d", r.runID, roomIndex, seat),
				name: fmt.Sprintf("Load %03d-%d", roomIndex, seat),
			}
		}
		roomID, err := r.createRoom(ctx, roomIndex, participants[0])
		if err != nil {
			return err
		}
		room := &testRoom{id: roomID, owner: participants[0], participants: participants, byID: make(map[string]*participant, len(participants))}
		for seat, player := range participants {
			room.byID[player.id] = player
			if seat > 0 {
				if err := r.post(ctx, "/api/v1/rooms/"+roomID+"/join", player, map[string]any{"seat": seat}, nil); err != nil {
					return fmt.Errorf("room %d join seat %d: %w", roomIndex, seat, err)
				}
			}
			command := clientCommand{Type: "room.ready", RequestID: fmt.Sprintf("%s-ready-%03d-%d", r.runID, roomIndex, seat), Payload: map[string]any{"ready": true}}
			if err := r.post(ctx, "/api/v1/rooms/"+roomID+"/commands", player, command, nil); err != nil {
				return fmt.Errorf("room %d ready seat %d: %w", roomIndex, seat, err)
			}
		}
		r.rooms[roomIndex] = room
		return nil
	})
}

func (r *runner) createRoom(ctx context.Context, roomIndex int, owner *participant) (string, error) {
	var response struct {
		ID string `json:"id"`
	}
	body := map[string]any{
		"name": fmt.Sprintf("Capacity %03d", roomIndex), "maxPlayers": 8, "blindPreset": "5/10", "actionSeconds": 45,
		"voiceEnabled": false, "chipDenominations": []int{5, 10, 20, 50, 100},
	}
	if err := r.post(ctx, "/api/v1/rooms", owner, body, &response); err != nil {
		return "", fmt.Errorf("room %d create: %w", roomIndex, err)
	}
	if response.ID == "" {
		return "", fmt.Errorf("room %d create returned an empty id", roomIndex)
	}
	return response.ID, nil
}

type socketPlan struct {
	room   *testRoom
	player *participant
}

func (r *runner) openSockets(ctx context.Context) error {
	plans := make([]socketPlan, 0, r.config.Connections)
	for _, room := range r.rooms {
		for _, player := range room.participants {
			plans = append(plans, socketPlan{room: room, player: player})
		}
	}
	uniqueConnections := len(plans)
	for index := uniqueConnections; index < r.config.Connections; index++ {
		room := r.rooms[(index-uniqueConnections)%len(r.rooms)]
		plans = append(plans, socketPlan{room: room, player: room.owner})
	}
	opened := make([]*socketClient, len(plans))
	err := parallel(ctx, len(plans), r.config.SocketConcurrency, func(ctx context.Context, index int) error {
		client, snapshot, err := r.dial(ctx, plans[index].room, plans[index].player)
		if err != nil {
			return fmt.Errorf("connection %d: %w", index, err)
		}
		if snapshot.RoomID != plans[index].room.id {
			client.closeNow()
			return fmt.Errorf("connection %d received snapshot for room %q", index, snapshot.RoomID)
		}
		opened[index] = client
		return nil
	})
	for index, client := range opened {
		if client == nil {
			continue
		}
		player := plans[index].player
		player.sockets = append(player.sockets, client)
		if player.primary == nil {
			player.primary = client
		}
		r.sockets = append(r.sockets, client)
	}
	return err
}

func (r *runner) startGames(ctx context.Context) error {
	return parallel(ctx, len(r.rooms), r.config.SetupConcurrency, func(ctx context.Context, index int) error {
		room := r.rooms[index]
		command := clientCommand{Type: "game.start", RequestID: fmt.Sprintf("%s-start-%03d", r.runID, index), Payload: map[string]any{}}
		if err := r.post(ctx, "/api/v1/rooms/"+room.id+"/commands", room.owner, command, nil); err != nil {
			return fmt.Errorf("room %d: %w", index, err)
		}
		return nil
	})
}

func (r *runner) measureActions(ctx context.Context) ([]time.Duration, error) {
	durations := make([]time.Duration, r.config.Actions)
	err := parallel(ctx, r.config.Actions, r.config.SetupConcurrency, func(ctx context.Context, index int) error {
		room := r.rooms[index]
		snapshot, err := r.snapshot(ctx, room, room.owner)
		if err != nil {
			return fmt.Errorf("room %d snapshot: %w", index, err)
		}
		actorID := ""
		for _, player := range snapshot.Players {
			if player.IsCurrentActor {
				actorID = player.ID
				break
			}
		}
		actor := room.byID[actorID]
		if actor == nil || actor.primary == nil {
			return fmt.Errorf("room %d has no connected current actor", index)
		}
		command := clientCommand{
			Type: "action.fold", RequestID: fmt.Sprintf("%s-action-%03d", r.runID, index),
			ExpectedVersion: snapshot.Version, Payload: map[string]any{},
		}
		duration, err := actor.primary.command(ctx, command, r.config.RequestTimeout)
		if err != nil {
			return fmt.Errorf("room %d: %w", index, err)
		}
		durations[index] = duration
		return nil
	})
	return durations, err
}

func (r *runner) measureReconnects(ctx context.Context) ([]time.Duration, error) {
	durations := make([]time.Duration, r.config.Reconnects)
	replacements := make([]*socketClient, r.config.Reconnects)
	err := parallel(ctx, r.config.Reconnects, r.config.SetupConcurrency, func(ctx context.Context, index int) error {
		room := r.rooms[index]
		player := room.participants[1]
		if player.primary == nil {
			return fmt.Errorf("room %d reconnect player has no primary connection", index)
		}
		if err := player.primary.closeGracefully(); err != nil {
			return fmt.Errorf("room %d close: %w", index, err)
		}
		disconnected, err := r.waitForStatus(ctx, room, player.id, "disconnected")
		if err != nil {
			return fmt.Errorf("room %d disconnect: %w", index, err)
		}
		started := time.Now()
		replacement, snapshot, err := r.dial(ctx, room, player)
		if err != nil {
			return fmt.Errorf("room %d redial: %w", index, err)
		}
		durations[index] = time.Since(started)
		if snapshot.Version <= disconnected.Version || playerStatus(snapshot, player.id) == "disconnected" {
			replacement.closeNow()
			return fmt.Errorf("room %d reconnect returned stale snapshot version %d after %d", index, snapshot.Version, disconnected.Version)
		}
		replacements[index] = replacement
		player.primary = replacement
		return nil
	})
	for _, replacement := range replacements {
		if replacement != nil {
			r.sockets = append(r.sockets, replacement)
		}
	}
	return durations, err
}

func (r *runner) waitForStatus(ctx context.Context, room *testRoom, userID, wanted string) (tableSnapshot, error) {
	deadline := time.Now().Add(r.config.RequestTimeout)
	for time.Now().Before(deadline) {
		snapshot, err := r.snapshot(ctx, room, room.owner)
		if err != nil {
			return tableSnapshot{}, err
		}
		if playerStatus(snapshot, userID) == wanted {
			return snapshot, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return tableSnapshot{}, ctx.Err()
		case <-timer.C:
		}
	}
	return tableSnapshot{}, fmt.Errorf("player %s did not reach status %s", userID, wanted)
}

func playerStatus(snapshot tableSnapshot, userID string) string {
	for _, player := range snapshot.Players {
		if player.ID == userID {
			return player.Status
		}
	}
	return ""
}

func (r *runner) snapshot(ctx context.Context, room *testRoom, player *participant) (tableSnapshot, error) {
	var snapshot tableSnapshot
	err := r.get(ctx, "/api/v1/rooms/"+room.id+"/snapshot", player, &snapshot)
	return snapshot, err
}

func (r *runner) dial(ctx context.Context, room *testRoom, player *participant) (*socketClient, tableSnapshot, error) {
	parsed, _ := url.Parse(r.config.Target)
	if parsed.Scheme == "https" {
		parsed.Scheme = "wss"
	} else {
		parsed.Scheme = "ws"
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/v1/rooms/" + room.id + "/events"
	requestContext, cancel := context.WithTimeout(ctx, r.config.RequestTimeout)
	defer cancel()
	connection, response, err := websocket.Dial(requestContext, parsed.String(), &websocket.DialOptions{HTTPHeader: identityHeaders(player)})
	if response != nil && response.Body != nil {
		defer response.Body.Close()
	}
	if err != nil {
		return nil, tableSnapshot{}, err
	}
	var initial envelope
	if err := wsjson.Read(requestContext, connection, &initial); err != nil {
		connection.CloseNow()
		return nil, tableSnapshot{}, err
	}
	if initial.Type != "table.snapshot" {
		connection.CloseNow()
		return nil, tableSnapshot{}, fmt.Errorf("first event was %q", initial.Type)
	}
	var snapshot tableSnapshot
	if err := json.Unmarshal(initial.Payload, &snapshot); err != nil {
		connection.CloseNow()
		return nil, tableSnapshot{}, err
	}
	readContext, readCancel := context.WithCancel(context.Background())
	client := &socketClient{connection: connection, cancel: readCancel, pending: make(map[string]chan socketResult)}
	go client.readLoop(readContext)
	return client, snapshot, nil
}

func (r *runner) get(ctx context.Context, path string, player *participant, destination any) error {
	return r.do(ctx, http.MethodGet, path, player, nil, destination)
}

func (r *runner) post(ctx context.Context, path string, player *participant, body, destination any) error {
	return r.do(ctx, http.MethodPost, path, player, body, destination)
}

func (r *runner) do(ctx context.Context, method, path string, player *participant, body, destination any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(r.config.Target, "/")+path, reader)
	if err != nil {
		return err
	}
	request.Header = identityHeaders(player)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := r.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return fmt.Errorf("%s %s returned %d: %s", method, path, response.StatusCode, strings.TrimSpace(string(raw)))
	}
	if destination == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(destination)
}

func identityHeaders(player *participant) http.Header {
	headers := make(http.Header)
	headers.Set("X-User-ID", player.id)
	headers.Set("X-User-Name", player.name)
	return headers
}

func (r *runner) close() {
	for _, client := range r.sockets {
		client.closeNow()
	}
	if transport, ok := r.httpClient.Transport.(*http.Transport); ok {
		transport.CloseIdleConnections()
	}
}

func (c *socketClient) readLoop(ctx context.Context) {
	for {
		var event envelope
		if err := wsjson.Read(ctx, c.connection, &event); err != nil {
			c.fail(err)
			return
		}
		if event.RequestID == "" {
			continue
		}
		c.mu.Lock()
		waiter := c.pending[event.RequestID]
		if waiter != nil {
			delete(c.pending, event.RequestID)
		}
		c.mu.Unlock()
		if waiter != nil {
			waiter <- socketResult{event: event}
		}
	}
}

func (c *socketClient) command(ctx context.Context, command clientCommand, timeout time.Duration) (time.Duration, error) {
	waiter := make(chan socketResult, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return 0, errors.New("websocket is closed")
	}
	c.pending[command.RequestID] = waiter
	c.mu.Unlock()

	commandContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	started := time.Now()
	c.writeMu.Lock()
	err := wsjson.Write(commandContext, c.connection, command)
	c.writeMu.Unlock()
	if err != nil {
		c.removePending(command.RequestID)
		return 0, err
	}
	select {
	case result := <-waiter:
		if result.err != nil {
			return 0, result.err
		}
		if result.event.Type == "error" {
			return 0, fmt.Errorf("server rejected command %s: %s", command.RequestID, string(result.event.Payload))
		}
		return time.Since(started), nil
	case <-commandContext.Done():
		c.removePending(command.RequestID)
		return 0, commandContext.Err()
	}
}

func (c *socketClient) removePending(requestID string) {
	c.mu.Lock()
	delete(c.pending, requestID)
	c.mu.Unlock()
}

func (c *socketClient) fail(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	waiters := make([]chan socketResult, 0, len(c.pending))
	for _, waiter := range c.pending {
		waiters = append(waiters, waiter)
	}
	c.pending = make(map[string]chan socketResult)
	c.mu.Unlock()
	for _, waiter := range waiters {
		waiter <- socketResult{err: err}
	}
}

func (c *socketClient) closeGracefully() error {
	err := c.connection.Close(websocket.StatusNormalClosure, "capacity reconnect")
	c.cancel()
	return err
}

func (c *socketClient) closeNow() {
	c.connection.CloseNow()
	c.cancel()
}

func uniqueParticipants(connections, rooms int) []int {
	counts := make([]int, rooms)
	for index := range counts {
		counts[index] = 2
	}
	remaining := min(connections, rooms*8) - rooms*2
	for remaining > 0 {
		for index := range counts {
			if remaining == 0 {
				break
			}
			if counts[index] < 8 {
				counts[index]++
				remaining--
			}
		}
	}
	return counts
}

func parallel(ctx context.Context, total, workers int, work func(context.Context, int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	jobs := make(chan int)
	var wait sync.WaitGroup
	var once sync.Once
	var firstErr error
	worker := func() {
		defer wait.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case index, open := <-jobs:
				if !open {
					return
				}
				if err := work(ctx, index); err != nil {
					once.Do(func() {
						firstErr = err
						cancel()
					})
					return
				}
			}
		}
	}
	workers = min(workers, total)
	wait.Add(workers)
	for range workers {
		go worker()
	}
	for index := 0; index < total; index++ {
		select {
		case jobs <- index:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
	}
	close(jobs)
	wait.Wait()
	if firstErr != nil {
		return firstErr
	}
	return ctx.Err()
}

func Percentile(values []time.Duration, percentile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 1 {
		return sorted[len(sorted)-1]
	}
	index := int(math.Ceil(percentile*float64(len(sorted)))) - 1
	return sorted[index]
}

func summarize(values []time.Duration, threshold time.Duration, evaluation string) LatencyMetrics {
	metrics := LatencyMetrics{Samples: len(values), ThresholdMs: milliseconds(threshold), Evaluation: evaluation}
	if len(values) == 0 {
		return metrics
	}
	metrics.MinimumMs = milliseconds(Percentile(values, 0))
	metrics.P50Ms = milliseconds(Percentile(values, 0.50))
	metrics.P95Ms = milliseconds(Percentile(values, 0.95))
	metrics.MaximumMs = milliseconds(Percentile(values, 1))
	if evaluation == "maximum" {
		metrics.Passed = Percentile(values, 1) <= threshold
	} else {
		metrics.Passed = Percentile(values, 0.95) <= threshold
	}
	return metrics
}

func milliseconds(value time.Duration) float64 {
	return math.Round(float64(value)/float64(time.Millisecond)*1000) / 1000
}

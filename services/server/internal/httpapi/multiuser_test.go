package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

type matrixAccount struct {
	ID      string
	Cookie  *http.Cookie
	Headers map[string]string
}

func TestMultiUserAccountRoomAndHandMatrix(t *testing.T) {
	application := New(Config{Development: false}, nil)
	t.Cleanup(application.Close)
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	client := server.Client()
	owner := registerMatrixAccount(t, client, server.URL, "13800138101", "矩阵房主")
	second := registerMatrixAccount(t, client, server.URL, "13800138102", "矩阵二号")
	third := registerMatrixAccount(t, client, server.URL, "13800138103", "矩阵三号")
	fourth := registerMatrixAccount(t, client, server.URL, "13800138104", "矩阵四号")
	roomBody := map[string]any{
		"name": "多人行为矩阵", "maxPlayers": 3, "blindPreset": "5/10", "actionSeconds": 20,
		"voiceEnabled": true, "chipDenominations": []int{5, 10, 20, 50, 100},
	}
	response := request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, owner.Headers)
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, response, &created)
	join := func(account matrixAccount, seat int, want int) {
		t.Helper()
		response := request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/join", map[string]any{"seat": seat}, account.Headers)
		if response.StatusCode != want {
			t.Fatalf("join %s at seat %d status = %d: %s", account.ID, seat, response.StatusCode, readBody(response))
		}
		response.Body.Close()
	}
	join(second, 1, http.StatusOK)
	join(third, 1, http.StatusConflict)
	join(third, 2, http.StatusOK)
	join(fourth, 2, http.StatusConflict)

	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, second.Headers)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("second active room creation status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/commands", map[string]any{
		"type": "room.rotate_invite", "requestId": "non-owner-rotate", "expectedVersion": 0, "payload": map[string]any{},
	}, second.Headers)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("non-owner management status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()

	ready := func(account matrixAccount, requestID string) {
		t.Helper()
		response := request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/commands", map[string]any{
			"type": "room.ready", "requestId": requestID, "expectedVersion": 0, "payload": map[string]any{"ready": true},
		}, account.Headers)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("ready %s status = %d: %s", account.ID, response.StatusCode, readBody(response))
		}
		response.Body.Close()
	}
	ready(owner, "ready-owner")
	ready(second, "ready-second")
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/commands", map[string]any{
		"type": "game.start", "requestId": "start-matrix", "expectedVersion": 0, "payload": map[string]any{},
	}, owner.Headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("start matrix hand status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()

	snapshots := map[string]room.TableSnapshot{}
	for _, account := range []matrixAccount{owner, second, third} {
		response := request(t, client, http.MethodGet, server.URL+"/api/v1/rooms/"+created.ID+"/snapshot", nil, account.Headers)
		var snapshot room.TableSnapshot
		decodeResponse(t, response, &snapshot)
		snapshots[account.ID] = snapshot
	}
	if len(snapshots[owner.ID].HoleCards) != 2 || len(snapshots[second.ID].HoleCards) != 2 || len(snapshots[third.ID].HoleCards) != 0 {
		t.Fatalf("private hand eligibility is wrong: owner=%d second=%d third=%d", len(snapshots[owner.ID].HoleCards), len(snapshots[second.ID].HoleCards), len(snapshots[third.ID].HoleCards))
	}
	for _, ownerCard := range snapshots[owner.ID].HoleCards {
		for _, secondCard := range snapshots[second.ID].HoleCards {
			if ownerCard == secondCard {
				t.Fatal("different players received overlapping private cards")
			}
		}
	}
	thirdPlayer := playerByID(t, snapshots[third.ID], third.ID)
	if thirdPlayer.StreetCommitted != 0 || thirdPlayer.TablePoints != 1000 {
		t.Fatalf("unready spectator paid into the hand: %#v", thirdPlayer)
	}

	actor := owner
	if playerByID(t, snapshots[second.ID], second.ID).IsCurrentActor {
		actor = second
	}
	actorSnapshot := snapshots[actor.ID]
	fold := map[string]any{
		"type": "action.fold", "requestId": "matrix-fold", "expectedVersion": actorSnapshot.Version, "payload": map[string]any{},
	}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/commands", fold, actor.Headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("current actor fold status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/commands", fold, actor.Headers)
	var duplicate struct {
		Duplicate bool `json:"duplicate"`
	}
	decodeResponse(t, response, &duplicate)
	if !duplicate.Duplicate {
		t.Fatal("retried poker action was applied twice")
	}
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/rooms/"+created.ID+"/snapshot", nil, owner.Headers)
	var settled room.TableSnapshot
	decodeResponse(t, response, &settled)
	if settled.Street != "settled" || settled.Pot != 0 {
		t.Fatalf("hand did not settle cleanly: street=%s pot=%d", settled.Street, settled.Pot)
	}
	var tablePoints int64
	for _, player := range settled.Players {
		tablePoints += player.TablePoints
	}
	if tablePoints != 3000 {
		t.Fatalf("table points were not conserved: %d", tablePoints)
	}

	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+created.ID+"/commands", map[string]any{
		"type": "room.leave", "requestId": "second-leaves", "expectedVersion": settled.Version, "payload": map[string]any{},
	}, second.Headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("second player leave status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, second.Headers)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("player could not create a room after leaving: %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
}

func TestJoiningAnotherRoomSwitchesMembershipAtomically(t *testing.T) {
	application := New(Config{Development: false}, nil)
	t.Cleanup(application.Close)
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	client := server.Client()
	switcher := registerMatrixAccount(t, client, server.URL, "13800138201", "换桌玩家")
	secondOwner := registerMatrixAccount(t, client, server.URL, "13800138202", "二桌房主")
	fullOwner := registerMatrixAccount(t, client, server.URL, "13800138203", "满桌房主")
	fullPlayer := registerMatrixAccount(t, client, server.URL, "13800138204", "满桌玩家")
	roomBody := map[string]any{
		"name": "切换矩阵", "maxPlayers": 2, "blindPreset": "5/10", "actionSeconds": 20,
		"voiceEnabled": true, "chipDenominations": []int{5, 10, 20, 50, 100},
	}
	create := func(account matrixAccount) string {
		t.Helper()
		response := request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, account.Headers)
		var created struct {
			ID string `json:"id"`
		}
		decodeResponse(t, response, &created)
		return created.ID
	}
	firstRoomID := create(switcher)
	secondRoomID := create(secondOwner)
	fullRoomID := create(fullOwner)
	response := request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+fullRoomID+"/join", map[string]any{"seat": 1}, fullPlayer.Headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fill target room status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+fullRoomID+"/join", map[string]any{"seat": 1}, switcher.Headers)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("full target switch status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/rooms/"+firstRoomID+"/snapshot", nil, switcher.Headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("failed switch did not preserve current room: %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	baseWSURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/rooms/" + firstRoomID
	switcherHeaders := http.Header{"Cookie": []string{switcher.Cookie.String()}}
	connections := make(map[string]*websocket.Conn)
	for _, name := range []string{"room-tab-one", "room-tab-two"} {
		connection, _, err := websocket.Dial(ctx, baseWSURL+"/events", &websocket.DialOptions{HTTPHeader: switcherHeaders})
		if err != nil {
			t.Fatal(err)
		}
		defer connection.CloseNow()
		var initial room.Envelope
		if err := wsjson.Read(ctx, connection, &initial); err != nil || initial.Type != "table.snapshot" {
			t.Fatalf("%s initial event = %#v, err = %v", name, initial, err)
		}
		connections[name] = connection
	}
	voiceConnection, _, err := websocket.Dial(ctx, baseWSURL+"/voice-events", &websocket.DialOptions{HTTPHeader: switcherHeaders})
	if err != nil {
		t.Fatal(err)
	}
	defer voiceConnection.CloseNow()
	var voiceInitial voiceServerEvent
	if err := wsjson.Read(ctx, voiceConnection, &voiceInitial); err != nil || voiceInitial.Type != "voice.peers" {
		t.Fatalf("voice initial event = %#v, err = %v", voiceInitial, err)
	}
	connections["voice"] = voiceConnection
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms/"+secondRoomID+"/join", map[string]any{"seat": 1}, switcher.Headers)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("switch target status = %d: %s", response.StatusCode, readBody(response))
	}
	var switched room.TableSnapshot
	decodeResponse(t, response, &switched)
	if switched.RoomID != secondRoomID || playerByID(t, switched, switcher.ID).Seat != 1 {
		t.Fatalf("switch response points at wrong membership: %#v", switched)
	}
	for name, connection := range connections {
		for {
			var event any
			err := wsjson.Read(ctx, connection, &event)
			if err == nil {
				continue
			}
			if status := websocket.CloseStatus(err); status != roomMembershipRevoked {
				t.Fatalf("%s switch close status = %d, err = %v", name, status, err)
			}
			break
		}
	}
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, switcher.Headers)
	var me struct {
		ActiveRoomID string `json:"activeRoomId"`
	}
	decodeResponse(t, response, &me)
	if me.ActiveRoomID != secondRoomID {
		t.Fatalf("account retained stale active room: %q", me.ActiveRoomID)
	}
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/rooms/"+firstRoomID+"/snapshot", nil, switcher.Headers)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("previous room still recognized switcher: %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
}

func registerMatrixAccount(t *testing.T, client *http.Client, baseURL, phone, nickname string) matrixAccount {
	t.Helper()
	response := request(t, client, http.MethodPost, baseURL+"/api/v1/auth/register", map[string]any{
		"phone": phone, "password": "table2026", "nickname": nickname,
	}, nil)
	if response.StatusCode != http.StatusCreated || len(response.Cookies()) != 1 {
		t.Fatalf("register %s status/cookie = %d/%d: %s", phone, response.StatusCode, len(response.Cookies()), readBody(response))
	}
	var registered struct {
		User struct {
			ID string `json:"id"`
		} `json:"user"`
	}
	cookie := response.Cookies()[0]
	decodeResponse(t, response, &registered)
	return matrixAccount{ID: registered.User.ID, Cookie: cookie, Headers: map[string]string{"Cookie": cookie.String()}}
}

func playerByID(t *testing.T, snapshot room.TableSnapshot, userID string) room.PlayerSnapshot {
	t.Helper()
	for _, player := range snapshot.Players {
		if player.ID == userID {
			return player
		}
	}
	t.Fatalf("player %s is missing from snapshot", userID)
	return room.PlayerSnapshot{}
}

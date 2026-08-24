package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

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

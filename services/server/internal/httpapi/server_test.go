package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/royal-flush/royal-flush/services/server/internal/room"
)

func TestHTTPScoreRoomAndAdminFlows(t *testing.T) {
	server := httptest.NewServer(New(Config{Development: true}, nil).Handler())
	defer server.Close()
	client := server.Client()

	response := request(t, client, http.MethodGet, server.URL+"/api/v1/health", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}
	response.Body.Close()

	roomBody := map[string]any{
		"name": "周五夜场", "maxPlayers": 8, "blindPreset": "5/10", "actionSeconds": 30,
		"voiceEnabled": true, "chipDenominations": []int{5, 10, 20, 50, 100, 500},
	}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, map[string]string{"X-User-ID": "u1", "X-User-Name": "小北"})
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create room status = %d: %s", response.StatusCode, readBody(response))
	}
	var created struct {
		ID     string      `json:"id"`
		Code   string      `json:"code"`
		Config room.Config `json:"config"`
	}
	decodeResponse(t, response, &created)
	if created.ID == "" || created.Code == "" || len(created.Config.ChipDenominations) != 6 {
		t.Fatalf("unexpected room response: %#v", created)
	}

	response = request(t, client, http.MethodGet, server.URL+"/api/v1/rooms/"+created.Code+"/public", nil, nil)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("public room status = %d", response.StatusCode)
	}
	response.Body.Close()

	addition := map[string]any{"amount": 250, "roomId": created.ID, "requestId": "add-1"}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/me/score-additions", addition, map[string]string{"X-User-ID": "u1"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("score addition status = %d: %s", response.StatusCode, readBody(response))
	}
	var scoreResult struct {
		Balance int64 `json:"balance"`
	}
	decodeResponse(t, response, &scoreResult)
	if scoreResult.Balance != 1250 {
		t.Fatalf("balance = %d", scoreResult.Balance)
	}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/me/score-additions", addition, map[string]string{"X-User-ID": "u1"})
	var retry struct {
		Balance int64 `json:"balance"`
	}
	decodeResponse(t, response, &retry)
	if retry.Balance != 1250 {
		t.Fatalf("idempotent retry balance = %d", retry.Balance)
	}

	reset := map[string]any{"reason": "测试新周期", "confirmation": "RESET ALL SCORES", "requestId": "reset-1"}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/admin/score-resets", reset, map[string]string{"X-User-ID": "u1"})
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("non-admin reset status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/admin/score-resets", reset, map[string]string{"X-User-ID": "admin", "X-Admin": "true"})
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin reset status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/admin/score-resets", reset, map[string]string{"X-User-ID": "admin", "X-Admin": "true"})
	var resetRetry struct {
		Duplicate bool `json:"duplicate"`
	}
	decodeResponse(t, response, &resetRetry)
	if !resetRetry.Duplicate {
		t.Fatal("reset retry should be reported as duplicate")
	}
}

func TestOTPAndWebSocketSnapshot(t *testing.T) {
	server := httptest.NewServer(New(Config{Development: true}, nil).Handler())
	defer server.Close()
	client := server.Client()
	response := request(t, client, http.MethodPost, server.URL+"/api/v1/auth/otp/request", map[string]any{"phone": "13800138000"}, nil)
	var challenge struct {
		DevCode string `json:"devCode"`
	}
	decodeResponse(t, response, &challenge)
	if challenge.DevCode != "123456" {
		t.Fatalf("dev code = %q", challenge.DevCode)
	}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/auth/otp/verify", map[string]any{"phone": "13800138000", "code": challenge.DevCode, "nickname": "小北"}, nil)
	if response.StatusCode != http.StatusOK || len(response.Cookies()) != 1 {
		t.Fatalf("verify status/cookie = %d/%d", response.StatusCode, len(response.Cookies()))
	}
	response.Body.Close()

	roomBody := map[string]any{
		"name": "语音测试桌", "maxPlayers": 2, "blindPreset": "5/10", "actionSeconds": 20,
		"voiceEnabled": true, "chipDenominations": []int{5, 10, 20, 50, 100},
	}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, map[string]string{"X-User-ID": "ws-user"})
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, response, &created)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/rooms/" + created.ID + "/events"
	connection, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: http.Header{"X-User-ID": []string{"ws-user"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(websocket.StatusNormalClosure, "test complete")
	var initial room.Envelope
	if err := wsjson.Read(ctx, connection, &initial); err != nil {
		t.Fatal(err)
	}
	if initial.Type != "table.snapshot" || initial.RoomID != created.ID {
		t.Fatalf("unexpected initial websocket event: %#v", initial)
	}
	command := room.ClientCommand{Type: "room.quick_message", RequestID: "quick-1", Payload: json.RawMessage(`{"message":"好牌"}`)}
	if err := wsjson.Write(ctx, connection, command); err != nil {
		t.Fatal(err)
	}
	var event room.Envelope
	if err := wsjson.Read(ctx, connection, &event); err != nil {
		t.Fatal(err)
	}
	if event.Type != "room.quick_message" || event.RequestID != "quick-1" {
		t.Fatalf("unexpected command event: %#v", event)
	}
}

func TestOperationsUserRoomReportAndAuditFlows(t *testing.T) {
	application := New(Config{Development: true}, nil)
	t.Cleanup(application.Close)
	server := httptest.NewServer(application.Handler())
	defer server.Close()
	client := server.Client()
	userHeaders := map[string]string{"X-User-ID": "u1", "X-User-Name": "玩家一"}
	adminHeaders := map[string]string{"X-User-ID": "admin", "X-User-Name": "运营", "X-Admin": "true"}

	response := request(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, userHeaders)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("seed user status = %d", response.StatusCode)
	}
	response.Body.Close()
	roomBody := map[string]any{
		"name": "举报测试房", "maxPlayers": 8, "blindPreset": "5/10", "actionSeconds": 30,
		"voiceEnabled": true, "chipDenominations": []int{5, 10, 20, 50, 100},
	}
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/rooms", roomBody, userHeaders)
	var created struct {
		ID string `json:"id"`
	}
	decodeResponse(t, response, &created)

	response = request(t, client, http.MethodPost, server.URL+"/api/v1/reports", map[string]any{
		"roomId": created.ID, "category": "conduct", "detail": "持续拖延操作", "requestId": "report-http-1",
	}, userHeaders)
	if response.StatusCode != http.StatusCreated {
		t.Fatalf("create report status = %d: %s", response.StatusCode, readBody(response))
	}
	var reportResult struct {
		Report struct {
			ID string `json:"id"`
		} `json:"report"`
	}
	decodeResponse(t, response, &reportResult)

	response = request(t, client, http.MethodGet, server.URL+"/api/v1/admin/rooms", nil, adminHeaders)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin rooms status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/admin/users?q=玩家", nil, adminHeaders)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("admin users status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/admin/reports/"+reportResult.Report.ID+"/resolution", map[string]any{
		"status": "resolved", "reason": "已核查", "requestId": "resolve-http-1",
	}, adminHeaders)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("resolve report status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodPost, server.URL+"/api/v1/admin/users/u1/ban-actions", map[string]any{
		"banned": true, "reason": "测试封禁", "requestId": "ban-http-1",
	}, adminHeaders)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("ban user status = %d: %s", response.StatusCode, readBody(response))
	}
	response.Body.Close()
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/me", nil, userHeaders)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("banned user status = %d", response.StatusCode)
	}
	response.Body.Close()
	response = request(t, client, http.MethodGet, server.URL+"/api/v1/admin/audit-log", nil, adminHeaders)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("audit log status = %d", response.StatusCode)
	}
	response.Body.Close()
}

func request(t *testing.T, client *http.Client, method, url string, body any, headers map[string]string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, destination any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(destination); err != nil {
		t.Fatal(err)
	}
}

func readBody(response *http.Response) string {
	defer response.Body.Close()
	raw, _ := io.ReadAll(response.Body)
	return string(raw)
}

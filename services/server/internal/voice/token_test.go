package voice

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMissingLiveKitFallsBackToWebRTC(t *testing.T) {
	token, err := (Config{}).Issue("u1", "小北", "room-1", time.Now())
	if err != nil || !token.Enabled || token.Transport != "webrtc" || len(token.ICEServers) == 0 {
		t.Fatalf("unexpected fallback response: %#v, %v", token, err)
	}
}

func TestIssueLiveKitCompatibleClaims(t *testing.T) {
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	token, err := (Config{URL: "wss://voice.example.test", APIKey: "key", APISecret: "secret"}).Issue("u1", "小北", "room-1", now)
	if err != nil || !token.Enabled || token.Transport != "livekit" {
		t.Fatal(err)
	}
	parts := strings.Split(token.AccessToken, ".")
	if len(parts) != 3 {
		t.Fatal("token is not a JWT")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		t.Fatal(err)
	}
	video := claims["video"].(map[string]any)
	if claims["sub"] != "u1" || video["room"] != "room-1" || video["roomJoin"] != true {
		t.Fatalf("unexpected claims: %#v", claims)
	}
}

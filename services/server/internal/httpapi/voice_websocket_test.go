package httpapi

import (
	"encoding/json"
	"testing"

	"github.com/royal-flush/royal-flush/services/server/internal/auth"
)

func TestVoiceHubReportsSignalBackpressure(t *testing.T) {
	hub := newVoiceHub()
	sender := &voiceClient{roomID: "room", user: auth.User{ID: "sender"}, send: make(chan voiceServerEvent, 1)}
	target := &voiceClient{roomID: "room", user: auth.User{ID: "target"}, send: make(chan voiceServerEvent, 1)}
	hub.rooms["room"] = map[string]map[*voiceClient]struct{}{
		"sender": {sender: {}},
		"target": {target: {}},
	}
	target.send <- voiceServerEvent{Type: "voice.peers"}
	message := voiceClientMessage{Type: "voice.candidate", TargetUserID: "target", Payload: json.RawMessage(`{"candidate":"test"}`)}
	if hub.relay(sender, message) {
		t.Fatal("relay reported success after every target queue was full")
	}
	<-target.send
	if !hub.relay(sender, message) {
		t.Fatal("relay failed after target queue capacity became available")
	}
}

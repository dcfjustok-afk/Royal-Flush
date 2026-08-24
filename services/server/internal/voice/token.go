package voice

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/royal-flush/royal-flush/services/server/internal/idgen"
)

type Config struct {
	URL        string
	APIKey     string
	APISecret  string
	ICEServers []ICEServer
}

type ICEServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

type Token struct {
	Enabled     bool        `json:"enabled"`
	Transport   string      `json:"transport,omitempty"`
	URL         string      `json:"url,omitempty"`
	AccessToken string      `json:"accessToken,omitempty"`
	ICEServers  []ICEServer `json:"iceServers,omitempty"`
	ExpiresAt   time.Time   `json:"expiresAt,omitempty"`
	Reason      string      `json:"reason,omitempty"`
}

func (c Config) Enabled() bool {
	return c.URL != "" && c.APIKey != "" && c.APISecret != ""
}

func (c Config) Issue(userID, nickname, roomID string, now time.Time) (Token, error) {
	if !c.Enabled() {
		iceServers := append([]ICEServer(nil), c.ICEServers...)
		if len(iceServers) == 0 {
			iceServers = []ICEServer{{URLs: []string{"stun:stun.l.google.com:19302"}}}
		}
		return Token{Enabled: true, Transport: "webrtc", ICEServers: iceServers, Reason: "使用浏览器直连语音"}, nil
	}
	if userID == "" || roomID == "" {
		return Token{}, errors.New("user and room are required")
	}
	jti, err := idgen.ID("voice")
	if err != nil {
		return Token{}, err
	}
	expiresAt := now.UTC().Add(15 * time.Minute)
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	payload := map[string]any{
		"iss": c.APIKey, "sub": userID, "name": nickname, "jti": jti,
		"nbf": now.UTC().Unix(), "exp": expiresAt.Unix(),
		"video": map[string]any{
			"roomJoin": true, "room": roomID, "canPublish": true,
			"canSubscribe": true, "canPublishData": true,
		},
	}
	encode := func(value any) (string, error) {
		raw, err := json.Marshal(value)
		if err != nil {
			return "", err
		}
		return base64.RawURLEncoding.EncodeToString(raw), nil
	}
	h, err := encode(header)
	if err != nil {
		return Token{}, err
	}
	p, err := encode(payload)
	if err != nil {
		return Token{}, err
	}
	unsigned := h + "." + p
	mac := hmac.New(sha256.New, []byte(c.APISecret))
	_, _ = mac.Write([]byte(unsigned))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return Token{Enabled: true, Transport: "livekit", URL: c.URL, AccessToken: unsigned + "." + signature, ExpiresAt: expiresAt}, nil
}

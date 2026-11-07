package slack

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/integrations/channel"
)

func TestRelaySignatureBindsBodyPathMethodAndTime(t *testing.T) {
	c := RelayCredential{Secret: strings.Repeat("s", 32)}
	now := time.Unix(1700000000, 0)
	body := []byte(`{"event":"test"}`)
	r := httptest.NewRequest("POST", "https://multica.example/api/slack/relay/install/events", nil)
	r.Header.Set("X-Relay-Timestamp", "1700000000")
	r.Header.Set("X-Relay-Nonce", strings.Repeat("a", 32))
	hash := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(c.Secret))
	fmt.Fprintf(mac, "v1\n1700000000\n%s\nPOST\n%s\n%x", strings.Repeat("a", 32), r.URL.Path, hash)
	r.Header.Set("X-Relay-Signature", hex.EncodeToString(mac.Sum(nil)))
	if !VerifyRelaySignature(c, r, body, now) {
		t.Fatal("valid signature rejected")
	}
	cases := map[string]func(*http.Request, *[]byte, *time.Time){
		"body":    func(_ *http.Request, b *[]byte, _ *time.Time) { *b = []byte("changed") },
		"path":    func(r *http.Request, _ *[]byte, _ *time.Time) { r.URL.Path += "/other" },
		"method":  func(r *http.Request, _ *[]byte, _ *time.Time) { r.Method = "GET" },
		"expired": func(_ *http.Request, _ *[]byte, at *time.Time) { *at = at.Add(301 * time.Second) },
		"future":  func(_ *http.Request, _ *[]byte, at *time.Time) { *at = at.Add(-301 * time.Second) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			request := r.Clone(t.Context())
			payload := append([]byte(nil), body...)
			at := now
			mutate(request, &payload, &at)
			if VerifyRelaySignature(c, request, payload, at) {
				t.Fatal("tampered request accepted")
			}
		})
	}
}

func TestRelayNormalizationAndAttachments(t *testing.T) {
	binding := RelayCredential{AppID: "ASUPPORT", TeamID: "TSUPPORT"}
	payload := `{"type":"event_callback","api_app_id":"ASUPPORT","team_id":"TSUPPORT","event":{"type":"message","channel":"CSUPPORT","user":"UREQUEST","ts":"100.200001","thread_ts":"100.100001","text":"<@UBOT001> evidence","files":[{"id":"FGOOD","name":"log.txt","mimetype":"text/plain","size":42,"url_private":"https://files.slack.com/files-pri/log.txt"},{"id":"FBAD","url_private":"https://attacker.example/log.txt"}]}}`
	event, err := ParseRelayEvent([]byte(payload), binding, "UBOT001")
	if err != nil || !event.Eligible || event.Message.Source.ThreadID != "100.100001" || event.Message.Text != "evidence" {
		t.Fatalf("normalization: %+v %v", event, err)
	}
	if !strings.Contains(string(event.Message.Raw), "FGOOD") || strings.Contains(string(event.Message.Raw), "FBAD") {
		t.Fatal("attachment allowlist failed")
	}
	binding.TeamID = "TOTHER00"
	if _, err = ParseRelayEvent([]byte(payload), binding, "UBOT001"); err == nil {
		t.Fatal("wrong team accepted")
	}
}

func TestRelayFactoryNeverRequiresOrOpensSocket(t *testing.T) {
	factory := newSlackFactory(ChannelDeps{})
	c, err := factory(channel.Config{Raw: []byte(`{"inbound_mode":"relay","app_id":"ASUPPORT","bot_token_encrypted":"eG94Yi10ZXN0"}`)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err = c.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = factory(channel.Config{Raw: []byte(`{"inbound_mode":"socket_mode","app_id":"ASUPPORT"}`)}); err == nil {
		t.Fatal("standalone must still require app token")
	}
	if _, err = factory(channel.Config{Raw: []byte(`{"inbound_mode":"invalid"}`)}); err == nil {
		t.Fatal("unknown mode opened a socket")
	}
}

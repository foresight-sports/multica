package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/integrations/slack"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

func relayFixture(t *testing.T) supportFixture {
	t.Helper()
	f := newSupportFixture(t)
	dbfx.Exec(t, `UPDATE channel_installation SET config=config||'{"inbound_mode":"relay","bot_user_id":"UBOT001"}' WHERE id=$1`, f.install)
	prior := testHandler.SlackRelayCredentials
	testHandler.SlackRelayCredentials = map[string]slack.RelayCredential{f.install: {InstallationID: f.install, WorkspaceID: testWorkspaceID, AppID: "ASUPPORT", TeamID: "TSUPPORT", Secret: strings.Repeat("s", 32)}}
	t.Cleanup(func() {
		testHandler.SlackRelayCredentials = prior
		testPool.Exec(context.Background(), `DELETE FROM slack_relay_health WHERE installation_id=$1`, f.install)
		testPool.Exec(context.Background(), `DELETE FROM slack_relay_receipt WHERE installation_id=$1`, f.install)
		testPool.Exec(context.Background(), `DELETE FROM slack_relay_nonce WHERE installation_id=$1`, f.install)
	})
	return f
}

func relayRequest(f supportFixture, typ, ch, ts, root string) *http.Request {
	body, _ := json.Marshal(map[string]any{"type": "event_callback", "api_app_id": "ASUPPORT", "team_id": "TSUPPORT", "event": map[string]any{"type": typ, "channel": ch, "ts": ts, "thread_ts": root, "user": "UREQUEST", "text": "private untrusted evidence", "files": []any{map[string]any{"id": "FLOG001", "name": "log.txt", "url_private": "https://files.slack.com/files-pri/log.txt"}}}})
	path := "/api/slack/relay/" + f.install + "/events"
	r := withURLParam(httptest.NewRequest("POST", path, strings.NewReader(string(body))), "installationId", f.install)
	stamp := fmt.Sprint(time.Now().Unix())
	nonce := strings.ReplaceAll(uuidToString(dbid.NewV7()), "-", "")
	sum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, []byte(strings.Repeat("s", 32)))
	fmt.Fprintf(mac, "v1\n%s\n%s\nPOST\n%s\n%x", stamp, nonce, path, sum)
	r.Header.Set("X-Relay-Timestamp", stamp)
	r.Header.Set("X-Relay-Nonce", nonce)
	r.Header.Set("X-Relay-Signature", hex.EncodeToString(mac.Sum(nil)))
	return r
}

func relayCall(t *testing.T, r *http.Request, want int) map[string]any {
	t.Helper()
	w := httptest.NewRecorder()
	testHandler.ReceiveSlackRelay(w, r)
	if w.Code != want {
		t.Fatalf("want %d got %d: %s", want, w.Code, w.Body.String())
	}
	var out map[string]any
	json.Unmarshal(w.Body.Bytes(), &out)
	return out
}

func TestSlackRelayAuthenticationAndReplayProtection(t *testing.T) {
	f := relayFixture(t)
	bad := relayRequest(f, "message", "CSUPPORT", "110.1", "")
	bad.Header.Set("X-Relay-Signature", "bad")
	relayCall(t, bad, 401)
	old := relayRequest(f, "message", "CSUPPORT", "110.1", "")
	old.Header.Set("X-Relay-Timestamp", "1")
	relayCall(t, old, 401)
	good := relayRequest(f, "message", "CSUPPORT", "110.1", "")
	duplicate := relayRequest(f, "message", "CSUPPORT", "110.1", "")
	duplicate.Header = good.Header.Clone()
	relayCall(t, good, 200)
	relayCall(t, duplicate, 409)
	// A fresh authenticated retry is accepted and deduplicated after restart.
	relayCall(t, relayRequest(f, "app_mention", "CSUPPORT", "110.1", ""), 200)
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM slack_relay_receipt WHERE installation_id=$1`, f.install).Scan(&n)
	if n != 1 {
		t.Fatalf("duplicate receipt: %d", n)
	}
	c := testHandler.SlackRelayCredentials[f.install]
	c.TeamID = "TWRONG00"
	testHandler.SlackRelayCredentials[f.install] = c
	relayCall(t, relayRequest(f, "message", "CSUPPORT", "111.1", ""), 409)
}

func TestSlackRelayOwnershipDisabledAndStatusPrivacy(t *testing.T) {
	f := relayFixture(t)
	dbfx.Exec(t, `UPDATE slack_intake_rule SET enabled=false WHERE id=$1`, f.rule)
	out := relayCall(t, relayRequest(f, "message", "CSUPPORT", "120.1", ""), 200)
	if out["owner"] != "multica" || out["state"] != "held_disabled" {
		t.Fatal(out)
	}
	out = relayCall(t, relayRequest(f, "app_mention", "CSUPPORT", "120.1", ""), 200)
	if out["owner"] != "multica" {
		t.Fatal("disabled workflow leaked to Onyx")
	}
	out = relayCall(t, relayRequest(f, "message", "COTHER00", "121.1", ""), 200)
	if out["owner"] != "onyx" {
		t.Fatal(out)
	}
	// Ownership cannot flip between duplicate event types after route changes.
	dbfx.Exec(t, `UPDATE slack_intake_rule SET channel_id='COTHER00',enabled=true WHERE id=$1`, f.rule)
	out = relayCall(t, relayRequest(f, "app_mention", "COTHER00", "121.1", ""), 200)
	if out["owner"] != "onyx" {
		t.Fatal("duplicate changed owners")
	}
	w := httptest.NewRecorder()
	testHandler.SlackRelayStatus(w, withURLParam(newRequest("GET", "/relay", nil), "id", testWorkspaceID))
	if w.Code != 200 || strings.Contains(w.Body.String(), "private untrusted") || strings.Contains(w.Body.String(), "files.slack.com") || strings.Contains(w.Body.String(), strings.Repeat("s", 32)) {
		t.Fatalf("unsafe status: %d", w.Code)
	}
}

func TestSlackRelayOutOfOrderRestartAndOneIssue(t *testing.T) {
	f := relayFixture(t)
	relayCall(t, relayRequest(f, "message", "CSUPPORT", "200.2", "200.1"), 200)
	var replyID string
	dbfx.QueryRow(t, `SELECT id::text FROM slack_relay_receipt WHERE installation_id=$1 AND message_ts='200.2'`, f.install).Scan(&replyID)
	if err := testHandler.processSlackRelayReceipt(t.Context(), parseUUID(replyID)); err != nil {
		t.Fatal(err)
	}
	var state string
	dbfx.QueryRow(t, `SELECT state FROM slack_relay_receipt WHERE id=$1`, replyID).Scan(&state)
	if state != "waiting_root" {
		t.Fatal(state)
	}
	relayCall(t, relayRequest(f, "app_mention", "CSUPPORT", "200.1", ""), 200)
	relayCall(t, relayRequest(f, "message", "CSUPPORT", "200.1", ""), 200)
	// Simulate a new worker instance: all progress is in Postgres.
	restarted := *testHandler
	var rootID string
	dbfx.QueryRow(t, `SELECT id::text FROM slack_relay_receipt WHERE installation_id=$1 AND message_ts='200.1'`, f.install).Scan(&rootID)
	if err := restarted.processSlackRelayReceipt(t.Context(), parseUUID(rootID)); err != nil {
		t.Fatal(err)
	}
	dbfx.Exec(t, `UPDATE slack_relay_receipt SET next_attempt_at=now() WHERE id=$1`, replyID)
	if err := restarted.processSlackRelayReceipt(t.Context(), parseUUID(replyID)); err != nil {
		t.Fatal(err)
	}
	if err := restarted.processSlackRelayReceipt(t.Context(), parseUUID(rootID)); err != nil {
		t.Fatal(err)
	}
	var threadID string
	var messages int
	dbfx.QueryRow(t, `SELECT id::text FROM slack_intake_thread WHERE rule_id=$1 AND root_ts='200.1'`, f.rule).Scan(&threadID)
	dbfx.QueryRow(t, `SELECT count(*) FROM slack_intake_message WHERE thread_id=$1`, threadID).Scan(&messages)
	if messages != 2 {
		t.Fatalf("messages=%d", messages)
	}
	var raw string
	dbfx.QueryRow(t, `SELECT raw::text FROM slack_intake_message WHERE thread_id=$1 ORDER BY message_ts LIMIT 1`, threadID).Scan(&raw)
	if !strings.Contains(raw, "FLOG001") {
		t.Fatal("attachment metadata missing")
	}
	if err := restarted.processSupportThread(t.Context(), parseUUID(threadID)); err != nil {
		t.Fatal(err)
	}
	var issues int
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE origin_id=$1`, threadID).Scan(&issues)
	if issues != 1 {
		t.Fatalf("issues=%d", issues)
	}
	// No live Slack or agent calls: the first pass only creates the durable task.
	var issueID string
	dbfx.QueryRow(t, `SELECT issue_id::text FROM slack_intake_thread WHERE id=$1`, threadID).Scan(&issueID)
	dbfx.Cleanup(t, `DELETE FROM agent_task_queue WHERE issue_id=$1`, issueID)
	dbfx.Cleanup(t, `DELETE FROM issue WHERE id=$1`, issueID)
}

func TestSlackRelayModeAndReplayAreHumanAdminOnly(t *testing.T) {
	f := relayFixture(t)
	r := withURLParam(withURLParam(newRequest("PUT", "/mode", map[string]string{"mode": "relay", "expected_mode": "relay"}), "id", testWorkspaceID), "installationId", f.install)
	chi.RouteContext(r.Context()).URLParams.Add("id", testWorkspaceID)
	r.Header.Set("X-Actor-Source", "task_token")
	testutil.Call(t, testHandler.SetSlackInboundMode, r).Want(403)
}

func TestSlackRelayReplayRequiresEnabledOriginalRouteAndRotationPreservesMode(t *testing.T) {
	f := relayFixture(t)
	dbfx.Exec(t, `UPDATE slack_intake_rule SET enabled=false WHERE id=$1`, f.rule)
	relayCall(t, relayRequest(f, "message", "CSUPPORT", "700.1", ""), 200)
	var receipt string
	dbfx.QueryRow(t, `SELECT id::text FROM slack_relay_receipt WHERE installation_id=$1`, f.install).Scan(&receipt)
	replay := func(source string) *http.Request {
		r := withURLParam(newRequest("POST", "/replay", nil), "receiptId", receipt)
		chi.RouteContext(r.Context()).URLParams.Add("id", testWorkspaceID)
		r.Header.Set("X-Actor-Source", source)
		return r
	}
	testutil.Call(t, testHandler.ReplaySlackRelay, replay("task_token")).Want(403)
	testutil.Call(t, testHandler.ReplaySlackRelay, replay("")).Want(409)
	dbfx.Exec(t, `UPDATE slack_intake_rule SET enabled=true WHERE id=$1`, f.rule)
	testutil.Call(t, testHandler.ReplaySlackRelay, replay("")).Want(200)
	mode := withURLParam(newRequest("PUT", "/mode", map[string]string{"mode": "socket_mode", "expected_mode": "relay"}), "installationId", f.install)
	chi.RouteContext(mode.Context()).URLParams.Add("id", testWorkspaceID)
	testutil.Call(t, testHandler.SetSlackInboundMode, mode).Want(409)
	inst, err := testHandler.Queries.GetChannelInstallation(t.Context(), db.GetChannelInstallationParams{ID: parseUUID(f.install), ChannelType: "slack"})
	if err != nil {
		t.Fatal(err)
	}
	var config map[string]any
	json.Unmarshal(inst.Config, &config)
	delete(config, "inbound_mode")
	raw, _ := json.Marshal(config)
	rotated, err := testHandler.Queries.UpsertChannelInstallation(t.Context(), db.UpsertChannelInstallationParams{WorkspaceID: inst.WorkspaceID, AgentID: inst.AgentID, ChannelType: "slack", Config: raw, InstallerUserID: inst.InstallerUserID})
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ID != inst.ID || slack.DecodePublicConfig(rotated.Config).InboundMode != "relay" {
		t.Fatal("credential rotation re-enabled a socket")
	}
}

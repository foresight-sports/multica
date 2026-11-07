package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// Explicitly opt in with the sibling checkout. This test uses only a local TLS
// server, synthetic Slack events, and the existing test database fixture.
func TestSlackRelayOnyxContractOneIssueNoCompetingAnswer(t *testing.T) {
	root := os.Getenv("ONYX_RELAY_TEST_ROOT")
	if root == "" {
		t.Skip("set ONYX_RELAY_TEST_ROOT to run the cross-repository Python/Go contract")
	}
	f := relayFixture(t)
	router := chi.NewRouter()
	router.Post("/api/slack/relay/{installationId}/events", testHandler.ReceiveSlackRelay)
	server := httptest.NewTLSServer(router)
	defer server.Close()
	certPath := filepath.Join(t.TempDir(), "test-ca.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.TLS.Certificates[0].Certificate[0]}), 0600); err != nil {
		t.Fatal(err)
	}
	config := map[string]any{"tenant_id": "public", "slack_bot_id": 1, "installation_id": f.install, "workspace_id": testWorkspaceID, "app_id": "ASUPPORT", "team_id": "TSUPPORT", "base_url": server.URL, "secret": strings.Repeat("s", 32)}
	dbURL := strings.Replace(os.Getenv("DATABASE_URL"), "postgres://", "postgresql+psycopg2://", 1)
	body, _ := json.Marshal(map[string]any{"config": config, "database_url": dbURL})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(root, ".venv", "bin", "python"), "-m", "onyx.db.tests.relay_contract_client")
	cmd.Env = append(os.Environ(), "PYTHONPATH="+filepath.Join(root, "backend"), "REQUESTS_CA_BUNDLE="+certPath)
	cmd.Stdin = bytes.NewReader(body)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("Python contract driver: %v\n%s", err, stderr.String())
	}
	var result struct {
		OnyxCalls int            `json:"onyx_calls"`
		Counts    map[string]int `json:"counts"`
	}
	if json.Unmarshal(output, &result) != nil || result.OnyxCalls != 0 || result.Counts["delivered"] != 2 {
		t.Fatalf("unexpected Onyx delivery summary: %s", output)
	}
	var receiptID string
	var n int
	dbfx.QueryRow(t, `SELECT count(*) FROM slack_relay_receipt WHERE installation_id=$1`, f.install).Scan(&n)
	if n != 1 {
		t.Fatalf("expected one Multica receipt, got %d", n)
	}
	dbfx.QueryRow(t, `SELECT id::text FROM slack_relay_receipt WHERE installation_id=$1`, f.install).Scan(&receiptID)
	if err = testHandler.processSlackRelayReceipt(t.Context(), parseUUID(receiptID)); err != nil {
		t.Fatal(err)
	}
	var threadID string
	dbfx.QueryRow(t, `SELECT id::text FROM slack_intake_thread WHERE rule_id=$1 AND root_ts='900.000001'`, f.rule).Scan(&threadID)
	if err = testHandler.processSupportThread(t.Context(), parseUUID(threadID)); err != nil {
		t.Fatal(err)
	}
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE origin_id=$1`, threadID).Scan(&n)
	if n != 1 {
		t.Fatalf("expected one issue, got %d", n)
	}
	// Replaying the already-committed worker receipt must not change issue count.
	if err = testHandler.processSlackRelayReceipt(t.Context(), parseUUID(receiptID)); err != nil {
		t.Fatal(err)
	}
	dbfx.QueryRow(t, `SELECT count(*) FROM issue WHERE origin_id=$1`, threadID).Scan(&n)
	if n != 1 {
		t.Fatal("replay created a second issue")
	}
	var issueID string
	dbfx.QueryRow(t, `SELECT issue_id::text FROM slack_intake_thread WHERE id=$1`, threadID).Scan(&issueID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM agent_task_queue WHERE issue_id=$1`, issueID)
		testPool.Exec(context.Background(), `DELETE FROM issue WHERE id=$1`, issueID)
	})
}

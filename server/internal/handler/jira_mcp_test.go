package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

func jiraTestHandler(t *testing.T, upstream http.HandlerFunc) *Handler {
	t.Helper()
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource/v2/mcp":
			json.NewEncoder(w).Encode(map[string]any{"resource": jiraMCPUpstream + "/v2/mcp", "authorization_servers": []string{origin + "/tenant"}})
		case "/.well-known/oauth-authorization-server/tenant":
			json.NewEncoder(w).Encode(map[string]string{"issuer": origin + "/tenant", "authorization_endpoint": origin + "/v1/authorize", "token_endpoint": origin + "/v1/token", "registration_endpoint": origin + "/v1/register"})
		default:
			upstream(w, r)
		}
	}))
	origin = server.URL
	t.Cleanup(server.Close)
	box, e := secretbox.New(make([]byte, 32))
	if e != nil {
		t.Fatal(e)
	}
	h := *testHandler
	h.JiraMCP = NewJiraMCP(box)
	h.JiraMCP.BaseURL = server.URL
	h.cfg.PublicURL = "https://multica.example"
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM jira_mcp_connection`)
		testPool.Exec(context.Background(), `DELETE FROM jira_mcp_session`)
	})
	return &h
}

func jiraSeedConnection(t *testing.T, h *Handler, user, token string, expiry time.Time) {
	t.Helper()
	sealed, e := h.JiraMCP.seal(jiraOAuthCredentials{ClientID: "client", AccessToken: token, RefreshToken: "refresh-old"})
	if e != nil {
		t.Fatal(e)
	}
	_, e = testPool.Exec(context.Background(), `INSERT INTO jira_mcp_connection(user_id,credentials,expires_at,status) VALUES($1,$2,$3,'connected') ON CONFLICT(user_id) DO UPDATE SET credentials=$2,expires_at=$3,status='connected'`, user, sealed, expiry)
	if e != nil {
		t.Fatal(e)
	}
}

func TestJiraMCPOAuthStatePKCEAndReplay(t *testing.T) {
	var verifier string
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/register":
			w.Write([]byte(`{"client_id":"client","token_endpoint_auth_method":"none"}`))
		case "/v1/token":
			r.ParseForm()
			verifier = r.Form.Get("code_verifier")
			w.Write([]byte(`{"access_token":"secret-access","refresh_token":"secret-refresh","expires_in":3600,"token_type":"Bearer"}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	})
	w := httptest.NewRecorder()
	connectRequest := newRequest("POST", jiraMCPPath+"/connect", nil)
	connectRequest.Header.Set("Referer", "https://multica.example/fs-app/issues/APP-9?tab=activity&view=full")
	h.JiraMCPConnect(w, connectRequest)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var link struct {
		URL string `json:"redirect_url"`
	}
	json.Unmarshal(w.Body.Bytes(), &link)
	u, _ := url.Parse(link.URL)
	state := u.Query().Get("state")
	// Callback cannot be substituted into a different browser.
	w = httptest.NewRecorder()
	h.JiraMCPCallback(w, httptest.NewRequest("GET", jiraMCPPath+"/callback?state="+state+"&code=test", nil))
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	w = httptest.NewRecorder()
	h.JiraMCPAuthorize(w, httptest.NewRequest("GET", link.URL, nil))
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	cookie := w.Result().Cookies()[0]
	form := url.Values{"state": {state}, "csrf": {cookie.Value}}
	r := httptest.NewRequest("POST", jiraMCPPath+"/authorize", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.AddCookie(cookie)
	w = httptest.NewRecorder()
	h.JiraMCPAuthorize(w, r)
	if w.Code != 303 {
		t.Fatal(w.Code, w.Body.String())
	}
	authURL, _ := url.Parse(w.Header().Get("Location"))
	if authURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatal("PKCE missing")
	}
	oauthCookie := w.Result().Cookies()[0]
	callback := func() *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", jiraMCPPath+"/callback?state="+state+"&code=test", nil)
		r.AddCookie(oauthCookie)
		w := httptest.NewRecorder()
		h.JiraMCPCallback(w, r)
		return w
	}
	w = callback()
	if w.Code != 200 || strings.Contains(w.Body.String(), "secret-") {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `href="/fs-app/issues/APP-9?tab=activity&amp;view=full"`) {
		t.Fatal("callback lost originating page")
	}
	if verifier == "" {
		t.Fatal("no PKCE verifier exchanged")
	}
	if callback().Code != 400 {
		t.Fatal("callback replay accepted")
	}
	var stored []byte
	if e := testPool.QueryRow(context.Background(), `SELECT credentials FROM jira_mcp_connection WHERE user_id=$1`, testUserID).Scan(&stored); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(stored), "secret-") {
		t.Fatal("plaintext credential stored")
	}
}

func TestJiraMCPRefreshSerializesRotation(t *testing.T) {
	var refreshes atomic.Int32
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		if r.Form.Get("refresh_token") != "refresh-old" {
			t.Error("wrong refresh token")
		}
		refreshes.Add(1)
		w.Write([]byte(`{"access_token":"new-access","refresh_token":"refresh-new","expires_in":3600,"token_type":"Bearer"}`))
	})
	jiraSeedConnection(t, h, testUserID, "old-access", time.Now().Add(-time.Minute))
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, e := h.jiraAccessToken(context.Background(), testUserID)
			if e != nil || token != "new-access" {
				t.Errorf("token=%q err=%v", token, e)
			}
		}()
	}
	wg.Wait()
	if refreshes.Load() != 1 {
		t.Fatalf("refreshes=%d", refreshes.Load())
	}
	var b []byte
	testPool.QueryRow(context.Background(), `SELECT credentials FROM jira_mcp_connection WHERE user_id=$1`, testUserID).Scan(&b)
	c, e := h.JiraMCP.open(b)
	if e != nil || c.RefreshToken != "refresh-new" {
		t.Fatal("rotation not persisted", e)
	}
}

func TestJiraMCPProxyUsesRequesterAndRevokesAccess(t *testing.T) {
	var calls atomic.Int32
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("tools") != "all" {
			t.Error("flat MCP catalog required")
		}
		if r.Header.Get("Authorization") != "Bearer requester-jira" {
			t.Errorf("wrong Jira identity: %s", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Cookie") != "" || r.Header.Get("CF-Access-Client-Secret") != "" {
			t.Error("unrelated credentials forwarded")
		}
		w.Header().Set("MCP-Session-Id", "upstream-secret-session")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}`))
	})
	requester := dbfx.Insert(t, "user", testutil.Cols{"name": "Jira requester", "email": "jira-requester@example.test"})
	dbfx.Insert(t, "member", testutil.Cols{"workspace_id": testWorkspaceID, "user_id": requester, "role": "member"})
	agent := createHandlerTestAgent(t, "JiraProxy", []byte("[]"))
	task := dbfx.Task(t, agent, testutil.Cols{"status": "running", "runtime_id": handlerTestRuntimeID(t), "originator_user_id": requester, "accountable_user_id": requester})
	otherTask := dbfx.Task(t, agent, testutil.Cols{"status": "running", "runtime_id": handlerTestRuntimeID(t), "originator_user_id": requester, "accountable_user_id": requester})
	jiraSeedConnection(t, h, requester, "requester-jira", time.Now().Add(time.Hour))
	jiraSeedConnection(t, h, testUserID, "machine-owner-jira", time.Now().Add(time.Hour))
	claim := AgentTaskResponse{ID: task, AuthToken: "mat_task_only", Agent: &TaskAgentData{}}
	h.attachJiraMCP(&claim)
	if !strings.Contains(string(claim.Agent.McpConfig), "mat_task_only") || strings.Contains(string(claim.Agent.McpConfig), "requester-jira") || strings.Contains(string(claim.Agent.McpConfig), "machine-owner-jira") {
		t.Fatal("claim leaked Jira credentials or omitted task-scoped access")
	}
	proxy := func(taskID, session string) *httptest.ResponseRecorder {
		r := newRequest("POST", jiraMCPPath+"/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
		r.Header.Set("X-Actor-Source", "task_token")
		r.Header.Set("X-Task-ID", taskID)
		r.Header.Set("X-Agent-ID", agent)
		r.Header.Set("X-Workspace-ID", testWorkspaceID)
		r.Header.Set("MCP-Session-Id", session)
		r.Header.Set("CF-Access-Client-Secret", "never-forward")
		w := httptest.NewRecorder()
		h.JiraMCPProxy(w, r)
		return w
	}
	w := proxy(task, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	session := w.Header().Get("MCP-Session-Id")
	if session == "" || session == "upstream-secret-session" {
		t.Fatal("session not isolated")
	}
	if proxy(otherTask, session).Code != 404 {
		t.Fatal("cross-task session accepted")
	}
	testPool.Exec(context.Background(), `UPDATE agent_task_queue SET status='completed' WHERE id=$1`, task)
	if proxy(task, session).Code != 403 {
		t.Fatal("completed task accepted")
	}
	r := newRequest("DELETE", jiraMCPPath, nil)
	r.Header.Set("X-User-ID", requester)
	w = httptest.NewRecorder()
	h.JiraMCPDisconnect(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	if result := proxy(otherTask, ""); result.Code != 200 || !strings.Contains(result.Body.String(), "multica-jira") {
		t.Fatal("disconnected requester did not receive the local connection gateway")
	}
	if calls.Load() != 1 {
		t.Fatalf("upstream calls=%d", calls.Load())
	}
}

func TestJiraMCPCatalogAndHumanBoundaries(t *testing.T) {
	for _, raw := range []string{`{"jsonrpc":"2.0","id":1,"result":{"tools":[{"name":"editJiraIssue"},{"name":"createConfluencePage"}]}}`, "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"editJiraIssue\"},{\"name\":\"createConfluencePage\"}]}}\n\n"} {
		filtered, e := filterJiraToolCatalog([]byte(raw))
		if e != nil || !strings.Contains(string(filtered), "editJiraIssue") || strings.Contains(string(filtered), "Confluence") {
			t.Fatal(string(filtered), e)
		}
	}
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected upstream request") })
	for _, handler := range []http.HandlerFunc{h.JiraMCPStatus, h.JiraMCPConnect, h.JiraMCPDisconnect} {
		r := newRequest("POST", jiraMCPPath, nil)
		r.Header.Set("X-Actor-Source", "task_token")
		w := httptest.NewRecorder()
		handler(w, r)
		if w.Code != 403 {
			t.Fatal("agent managed personal connection", w.Code)
		}
	}
}

func TestJiraMCPLateUnauthorizedDoesNotRevokeNewConnection(t *testing.T) {
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected request") })
	jiraSeedConnection(t, h, testUserID, "new-token", time.Now().Add(time.Hour))
	h.invalidateJiraToken(context.Background(), testUserID, "old-token")
	if token, e := h.jiraAccessToken(context.Background(), testUserID); e != nil || token != "new-token" {
		t.Fatal(token, e)
	}
}

func TestJiraMCPRefreshFailureRequiresReconnect(t *testing.T) {
	h := jiraTestHandler(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		w.Write([]byte(`{"error":"invalid_grant"}`))
	})
	jiraSeedConnection(t, h, testUserID, "expired", time.Now().Add(-time.Hour))
	if _, e := h.jiraAccessToken(context.Background(), testUserID); e == nil {
		t.Fatal("refresh failure accepted")
	}
	var status string
	testPool.QueryRow(context.Background(), `SELECT status FROM jira_mcp_connection WHERE user_id=$1`, testUserID).Scan(&status)
	if status != "reconnect_required" {
		t.Fatal(status)
	}
}

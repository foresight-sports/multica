package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/multica-ai/multica/server/internal/util/secretbox"
)

const jiraMCPUpstream = "https://mcp.atlassian.com"
const jiraMCPPath = "/api/integrations/jira"

// JiraMCP is deliberately separate from owner-scoped shared integrations.
// OAuth tokens never enter daemon configuration, prompts, or task records.
type JiraMCP struct {
	Box     *secretbox.Box
	Client  *http.Client
	BaseURL string
}

func NewJiraMCP(box *secretbox.Box) *JiraMCP {
	return &JiraMCP{Box: box, BaseURL: jiraMCPUpstream, Client: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
}

type jiraOAuthCredentials struct {
	ClientID              string `json:"client_id"`
	ReturnPath            string `json:"return_path,omitempty"`
	AuthorizationEndpoint string `json:"authorization_endpoint,omitempty"`
	TokenEndpoint         string `json:"token_endpoint,omitempty"`
	Scope                 string `json:"scope,omitempty"`
	Verifier              string `json:"verifier,omitempty"`
	AccessToken           string `json:"access_token,omitempty"`
	RefreshToken          string `json:"refresh_token,omitempty"`
}

func jiraRandom() (string, error) {
	b := make([]byte, 32)
	_, err := rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), err
}
func jiraHash(s string) string { sum := sha256.Sum256([]byte(s)); return hex.EncodeToString(sum[:]) }
func (j *JiraMCP) seal(c jiraOAuthCredentials) ([]byte, error) {
	b, e := json.Marshal(c)
	if e != nil {
		return nil, e
	}
	return j.Box.Seal(b)
}
func (j *JiraMCP) open(b []byte) (jiraOAuthCredentials, error) {
	var c jiraOAuthCredentials
	p, e := j.Box.Open(b)
	if e == nil {
		e = json.Unmarshal(p, &c)
	}
	return c, e
}

// OAuth error bodies may contain credentials. Return only bounded status classes.
func (j *JiraMCP) request(ctx context.Context, path, contentType string, body []byte, out any) error {
	endpoint := path
	if strings.HasPrefix(path, "/") {
		endpoint = j.BaseURL + path
	}
	if !j.trustedOAuthURL(endpoint) {
		return errors.New("Untrusted Atlassian OAuth endpoint")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", contentType)
	resp, e := j.Client.Do(req)
	if e != nil {
		return errors.New("Atlassian connection unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if resp.StatusCode == 400 || resp.StatusCode == 401 {
			return errJiraReconnect
		}
		return errors.New("Atlassian authorization unavailable")
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}

var errJiraReconnect = errors.New("Reconnect Jira in your Multica account settings")

func (h *Handler) JiraMCPStatus(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "Personal Jira connections require a human account")
		return
	}
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	status := "disconnected"
	pending := false
	if h.JiraMCP == nil {
		writeJSON(w, 200, map[string]any{"configured": false, "status": status, "pending": false})
		return
	}
	err := h.DB.QueryRow(r.Context(), `SELECT status, COALESCE(pending_until > now(),false) FROM jira_mcp_connection WHERE user_id=$1`, user).Scan(&status, &pending)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "Could not load Jira connection")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"configured": true, "status": status, "pending": pending})
}

func (h *Handler) JiraMCPConnect(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "Personal Jira connections require a human account")
		return
	}
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	j := h.JiraMCP
	if j == nil || !strings.HasPrefix(h.cfg.PublicURL, "https://") {
		writeError(w, 503, "Jira connection is not configured")
		return
	}
	state, e := jiraRandom()
	if e != nil {
		writeError(w, 500, "Could not begin Jira sign-in")
		return
	}
	verifier, e := jiraRandom()
	if e != nil {
		writeError(w, 500, "Could not begin Jira sign-in")
		return
	}
	metadata, e := j.discoverOAuth(r.Context())
	if e != nil {
		writeError(w, 502, "Could not discover Atlassian sign-in; retry shortly")
		return
	}
	callback := strings.TrimRight(h.cfg.PublicURL, "/") + jiraMCPPath + "/callback"
	registration, _ := json.Marshal(map[string]any{"client_name": "Multica", "redirect_uris": []string{callback}, "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}, "token_endpoint_auth_method": "none"})
	var client struct {
		ID     string `json:"client_id"`
		Method string `json:"token_endpoint_auth_method"`
	}
	if e = j.request(r.Context(), metadata.Registration, "application/json", registration, &client); e != nil || client.ID == "" || (client.Method != "" && client.Method != "none") {
		writeError(w, 502, "Could not register Multica with Atlassian")
		return
	}
	sealed, e := j.seal(jiraOAuthCredentials{ClientID: client.ID, Verifier: verifier, ReturnPath: jiraReturnPath(h.cfg.PublicURL, r.Referer()), AuthorizationEndpoint: metadata.Authorization, TokenEndpoint: metadata.Token, Scope: "read:me read:account offline_access read:jira:agent-interface write:jira:agent-interface search:jira:agent-interface delete:jira:agent-interface manage:jira:agent-interface"})
	if e != nil {
		writeError(w, 500, "Could not secure Jira sign-in")
		return
	}
	_, e = h.DB.Exec(r.Context(), `INSERT INTO jira_mcp_connection(user_id,pending_state,pending_credentials,pending_until) VALUES($1,$2,$3,now()+interval '10 minutes') ON CONFLICT(user_id) DO UPDATE SET pending_state=EXCLUDED.pending_state,pending_credentials=EXCLUDED.pending_credentials,pending_until=EXCLUDED.pending_until,updated_at=now()`, user, jiraHash(state), sealed)
	if e != nil {
		writeError(w, 500, "Could not save Jira sign-in")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]string{"redirect_url": strings.TrimRight(h.cfg.PublicURL, "/") + jiraMCPPath + "/authorize?state=" + url.QueryEscape(state)})
}

// The browser confirmation binds consent to the named Multica account and
// establishes cookies even when desktop initiated the flow in another process.
func (h *Handler) JiraMCPAuthorize(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; "+jiraAuthFormAction+"; frame-ancestors 'none'")
	if h.JiraMCP == nil {
		h.jiraAuthError(w, 503, "Jira is not configured")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if e := r.ParseForm(); e != nil {
		h.jiraAuthError(w, 400, "Invalid sign-in")
		return
	}
	state := r.Form.Get("state")
	var email string
	var sealed []byte
	if len(state) < 32 {
		h.jiraAuthError(w, 400, "Invalid sign-in")
		return
	}
	e := h.DB.QueryRow(r.Context(), `SELECT u.email,j.pending_credentials FROM jira_mcp_connection j JOIN "user" u ON u.id=j.user_id WHERE j.pending_state=$1 AND j.pending_until>now()`, jiraHash(state)).Scan(&email, &sealed)
	if e != nil {
		h.jiraAuthError(w, 400, "Jira sign-in expired. Start again in Multica.")
		return
	}
	c, e := h.JiraMCP.open(sealed)
	if e != nil {
		h.jiraAuthError(w, 500, "Could not read sign-in")
		return
	}
	if r.Method == http.MethodGet {
		nonce, e := jiraRandom()
		if e != nil {
			h.jiraAuthError(w, 500, "Could not begin sign-in")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "multica_jira_start", Value: nonce, Path: jiraMCPPath, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		h.jiraAuthPage(w, http.StatusOK, jiraAuthPageData{Title: "Connect your Jira account", Description: "Bring your Jira work into the loop. Sign in to let your agents work alongside you.", Email: email, State: state, Nonce: nonce, ReturnURL: jiraReturnPath(h.cfg.PublicURL, c.ReturnPath)})
		return
	}
	cookie, e := r.Cookie("multica_jira_start")
	if e != nil || cookie.Value == "" || cookie.Value != r.Form.Get("csrf") {
		h.jiraAuthError(w, 403, "Reload the sign-in page and try again")
		return
	}
	challenge := sha256.Sum256([]byte(c.Verifier))
	q := url.Values{"response_type": {"code"}, "client_id": {c.ClientID}, "redirect_uri": {strings.TrimRight(h.cfg.PublicURL, "/") + jiraMCPPath + "/callback"}, "state": {state}, "code_challenge": {base64.RawURLEncoding.EncodeToString(challenge[:])}, "code_challenge_method": {"S256"}, "resource": {jiraMCPUpstream + "/v2/mcp"}}
	http.SetCookie(w, &http.Cookie{Name: "multica_jira_oauth", Value: state, Path: jiraMCPPath, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	http.SetCookie(w, &http.Cookie{Name: "multica_jira_start", Value: "", Path: jiraMCPPath, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	endpoint := c.AuthorizationEndpoint
	if endpoint == "" {
		endpoint = h.JiraMCP.BaseURL + "/v1/authorize"
	}
	if !h.JiraMCP.trustedOAuthURL(endpoint) {
		h.jiraAuthError(w, 502, "Invalid Atlassian sign-in endpoint")
		return
	}
	if c.Scope != "" {
		q.Set("scope", c.Scope)
	}
	http.Redirect(w, r, endpoint+"?"+q.Encode(), http.StatusSeeOther)
}

func (h *Handler) JiraMCPCallback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	state := r.URL.Query().Get("state")
	cookie, e := r.Cookie("multica_jira_oauth")
	if h.JiraMCP == nil || e != nil || len(state) < 32 || cookie.Value != state {
		h.jiraAuthError(w, 400, "Invalid or expired Jira sign-in. Start again in account settings.")
		return
	}
	tx, e := h.TxStarter.Begin(r.Context())
	if e != nil {
		h.jiraAuthError(w, 500, "Could not finish Jira sign-in")
		return
	}
	defer tx.Rollback(r.Context())
	var user string
	var sealed []byte
	e = tx.QueryRow(r.Context(), `SELECT user_id,pending_credentials FROM jira_mcp_connection WHERE pending_state=$1 AND pending_until>now() FOR UPDATE`, jiraHash(state)).Scan(&user, &sealed)
	if e != nil {
		h.jiraAuthError(w, 400, "Jira sign-in expired or was already used")
		return
	}
	c, e := h.JiraMCP.open(sealed)
	if e != nil {
		h.jiraAuthError(w, 500, "Could not read Jira sign-in")
		return
	}
	// Consume even a denied/failed callback; retries require fresh consent state.
	_, e = tx.Exec(r.Context(), `UPDATE jira_mcp_connection SET pending_state=NULL,pending_credentials=NULL,pending_until=NULL WHERE user_id=$1`, user)
	if e != nil {
		h.jiraAuthError(w, 500, "Could not finish Jira sign-in")
		return
	}
	code := r.URL.Query().Get("code")
	var expires time.Time
	if code == "" || r.URL.Query().Get("error") != "" {
		e = errJiraReconnect
	} else {
		c, expires, e = h.JiraMCP.exchange(r.Context(), c, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {strings.TrimRight(h.cfg.PublicURL, "/") + jiraMCPPath + "/callback"}, "code_verifier": {c.Verifier}})
	}
	if e != nil {
		_ = tx.Commit(r.Context())
		h.jiraAuthPage(w, 400, jiraAuthPageData{Title: "Let’s reconnect", Description: "Jira sign-in was not completed. Return to Multica to try again.", ReturnURL: jiraReturnPath(h.cfg.PublicURL, c.ReturnPath)})
		return
	}
	sealed, e = h.JiraMCP.seal(c)
	if e != nil {
		h.jiraAuthError(w, 500, "Could not secure Jira connection")
		return
	}
	_, e = tx.Exec(r.Context(), `UPDATE jira_mcp_connection SET credentials=$2,expires_at=$3,status='connected',updated_at=now() WHERE user_id=$1`, user, sealed, expires)
	if e == nil {
		_, e = tx.Exec(r.Context(), `DELETE FROM jira_mcp_session WHERE user_id=$1`, user)
	}
	if e != nil || tx.Commit(r.Context()) != nil {
		h.jiraAuthError(w, 500, "Could not save Jira connection")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "multica_jira_oauth", Value: "", Path: jiraMCPPath, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
	h.jiraAuthPage(w, http.StatusOK, jiraAuthPageData{Title: "Jira is connected", Description: "Your new agent runs can now use your Jira account, with your permissions.", Connected: true, ReturnURL: jiraReturnPath(h.cfg.PublicURL, c.ReturnPath)})
}

func (j *JiraMCP) exchange(ctx context.Context, c jiraOAuthCredentials, form url.Values) (jiraOAuthCredentials, time.Time, error) {
	form.Set("client_id", c.ClientID)
	form.Set("resource", jiraMCPUpstream+"/v2/mcp")
	var token struct {
		Access  string `json:"access_token"`
		Refresh string `json:"refresh_token"`
		Expires int64  `json:"expires_in"`
		Type    string `json:"token_type"`
	}
	endpoint := c.TokenEndpoint
	if endpoint == "" {
		endpoint = "/v1/token"
	}
	if e := j.request(ctx, endpoint, "application/x-www-form-urlencoded", []byte(form.Encode()), &token); e != nil {
		return c, time.Time{}, e
	}
	if token.Access == "" || token.Expires <= 0 || !strings.EqualFold(token.Type, "Bearer") {
		return c, time.Time{}, errJiraReconnect
	}
	c.AccessToken = token.Access
	c.Verifier = ""
	if token.Refresh != "" {
		c.RefreshToken = token.Refresh
	}
	return c, time.Now().Add(time.Duration(token.Expires) * time.Second), nil
}

func (h *Handler) JiraMCPDisconnect(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "Personal Jira connections require a human account")
		return
	}
	user, ok := requireUserID(w, r)
	if !ok {
		return
	}
	tx, e := h.TxStarter.Begin(r.Context())
	if e != nil {
		writeError(w, 500, "Could not disconnect Jira")
		return
	}
	defer tx.Rollback(r.Context())
	_, e = tx.Exec(r.Context(), `DELETE FROM jira_mcp_connection WHERE user_id=$1`, user)
	if e == nil {
		_, e = tx.Exec(r.Context(), `DELETE FROM jira_mcp_session WHERE user_id=$1`, user)
	}
	if e == nil {
		e = tx.Commit(r.Context())
	}
	if e != nil {
		writeError(w, 500, "Could not disconnect Jira")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Refresh under a row lock: rotating refresh tokens must have exactly one user.
func (h *Handler) jiraAccessToken(ctx context.Context, user string) (string, error) {
	tx, e := h.TxStarter.Begin(ctx)
	if e != nil {
		return "", e
	}
	defer tx.Rollback(ctx)
	var sealed []byte
	var expires time.Time
	e = tx.QueryRow(ctx, `SELECT credentials,expires_at FROM jira_mcp_connection WHERE user_id=$1 AND status='connected' FOR UPDATE`, user).Scan(&sealed, &expires)
	if errors.Is(e, pgx.ErrNoRows) {
		return "", errJiraReconnect
	}
	if e != nil {
		return "", e
	}
	c, e := h.JiraMCP.open(sealed)
	if e != nil {
		return "", errJiraReconnect
	}
	if time.Until(expires) < time.Minute {
		if c.RefreshToken == "" {
			e = errJiraReconnect
		} else {
			c, expires, e = h.JiraMCP.exchange(ctx, c, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {c.RefreshToken}})
		}
		if e != nil {
			if errors.Is(e, errJiraReconnect) {
				_, _ = tx.Exec(ctx, `UPDATE jira_mcp_connection SET status='reconnect_required',credentials=NULL WHERE user_id=$1`, user)
				_ = tx.Commit(ctx)
			}
			return "", e
		}
		sealed, e = h.JiraMCP.seal(c)
		if e != nil {
			return "", e
		}
		if _, e = tx.Exec(ctx, `UPDATE jira_mcp_connection SET credentials=$2,expires_at=$3,updated_at=now() WHERE user_id=$1`, user, sealed, expires); e != nil {
			return "", e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return "", e
	}
	return c.AccessToken, nil
}

// Task tokens determine the task only. Their UserID is the machine owner, NOT
// the Jira principal. Resolve the requester exclusively from the persisted task.
func (h *Handler) JiraMCPProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Header.Get("X-Actor-Source") != "task_token" || h.JiraMCP == nil {
		writeError(w, 403, "A running Multica task is required")
		return
	}
	var user string
	e := h.DB.QueryRow(r.Context(), `SELECT t.originator_user_id::text FROM agent_task_queue t JOIN agent a ON a.id=t.agent_id JOIN member m ON m.workspace_id=a.workspace_id AND m.user_id=t.originator_user_id WHERE t.id=$1 AND t.agent_id=$2 AND a.workspace_id=$3 AND t.status='running' AND t.originator_user_id IS NOT NULL`, r.Header.Get("X-Task-ID"), r.Header.Get("X-Agent-ID"), r.Header.Get("X-Workspace-ID")).Scan(&user)
	if e != nil {
		writeError(w, 403, "This task has no authorized Jira requester")
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		writeError(w, 413, "MCP request too large")
		return
	}
	// Once this run has asked the human to connect, OAuth alone must not let
	// its next tool call restart Jira work. The explicit Resume creates a new
	// task with its own gateway identity.
	var blocked bool
	if e = h.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM inbox_item WHERE id=$1 AND recipient_id=$2 AND details->>'reason'='jira_connection_required')`, jiraBlockerID(r.Header.Get("X-Task-ID")), user).Scan(&blocked); e != nil {
		writeError(w, 503, "Could not check Jira blocker; retry shortly")
		return
	}
	var localCall struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	_ = json.Unmarshal(body, &localCall)
	if blocked || (localCall.Method == "tools/call" && localCall.Params.Name == "jira_connection_required") {
		h.jiraConnectionRequired(w, r, body, user)
		return
	}
	token, e := h.jiraAccessToken(r.Context(), user)
	if errors.Is(e, errJiraReconnect) {
		h.jiraConnectionRequired(w, r, body, user)
		return
	}
	if e != nil {
		writeError(w, 503, "Could not check Jira connection; retry shortly")
		return
	}
	// Use Atlassian's flat catalog so deferred Jira tools remain available
	// without exposing generic execute tools that can invoke other products.
	req, e := http.NewRequestWithContext(r.Context(), r.Method, h.JiraMCP.BaseURL+"/v2/mcp?tools=all", bytes.NewReader(body))
	if e != nil {
		writeError(w, 500, "Could not create Jira request")
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	for _, name := range []string{"Content-Type", "Accept", "MCP-Protocol-Version", "Last-Event-ID"} {
		if v := r.Header.Get(name); v != "" {
			req.Header.Set(name, v)
		}
	}
	localSession := r.Header.Get("MCP-Session-Id")
	if localSession != "" {
		var sealed []byte
		if e = h.DB.QueryRow(r.Context(), `SELECT upstream_id FROM jira_mcp_session WHERE id=$1 AND task_id=$2 AND user_id=$3 AND expires_at>now()`, localSession, r.Header.Get("X-Task-ID"), user).Scan(&sealed); e != nil {
			writeError(w, 404, "MCP session expired; initialize again")
			return
		}
		upstream, e := h.JiraMCP.Box.Open(sealed)
		if e != nil {
			writeError(w, 500, "Could not read MCP session")
			return
		}
		req.Header.Set("MCP-Session-Id", string(upstream))
	}
	var rpc struct {
		Method string `json:"method"`
		Params struct {
			Name string `json:"name"`
		} `json:"params"`
	}
	if r.Method == http.MethodPost && json.Unmarshal(body, &rpc) != nil {
		writeError(w, 400, "Invalid MCP request")
		return
	}
	if r.Method == http.MethodPost {
		switch rpc.Method {
		case "initialize", "notifications/initialized", "notifications/cancelled", "ping", "tools/list", "tools/call":
		default:
			writeError(w, 403, "MCP method is not available for Jira")
			return
		}
	}
	if rpc.Method == "tools/call" && !jiraMCPToolAllowed(rpc.Params.Name) {
		writeError(w, 403, "This connection exposes Jira tools only")
		return
	}
	resp, e := h.JiraMCP.Client.Do(req)
	if e != nil {
		writeError(w, 502, "Jira MCP is unavailable")
		return
	}
	defer resp.Body.Close()
	if rpc.Method == "tools/call" {
		slog.Info("jira MCP tool request", "task_id", r.Header.Get("X-Task-ID"), "requester_id", user, "tool", rpc.Params.Name, "http_status", resp.StatusCode)
	}
	if resp.StatusCode == 401 {
		h.invalidateJiraToken(r.Context(), user, token)
		h.jiraConnectionRequired(w, r, body, user)
		return
	}
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		writeError(w, 502, "Unexpected Jira MCP redirect")
		return
	}
	if upstream := resp.Header.Get("MCP-Session-Id"); upstream != "" {
		if localSession == "" {
			localSession, e = jiraRandom()
			if e != nil {
				writeError(w, 500, "Could not create MCP session")
				return
			}
		}
		sealed, e := h.JiraMCP.Box.Seal([]byte(upstream))
		if e != nil {
			writeError(w, 500, "Could not secure MCP session")
			return
		}
		_, e = h.DB.Exec(r.Context(), `INSERT INTO jira_mcp_session(id,task_id,user_id,upstream_id) VALUES($1,$2,$3,$4) ON CONFLICT(id) DO UPDATE SET upstream_id=EXCLUDED.upstream_id`, localSession, r.Header.Get("X-Task-ID"), user, sealed)
		if e != nil {
			writeError(w, 500, "Could not save MCP session")
			return
		}
		w.Header().Set("MCP-Session-Id", localSession)
	}
	for _, name := range []string{"Content-Type", "MCP-Protocol-Version"} {
		if v := resp.Header.Get(name); v != "" {
			w.Header().Set(name, v)
		}
	}
	if rpc.Method == "tools/list" && resp.StatusCode == 200 {
		data, e := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		if e != nil || len(data) > 4<<20 {
			writeError(w, 502, "Invalid Jira tool catalog")
			return
		}
		filtered, e := filterJiraToolCatalog(data)
		if e != nil {
			writeError(w, 502, "Invalid Jira tool catalog")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(filtered)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	// Stream event responses without buffering; cap response size defensively.
	buf := make([]byte, 32<<10)
	remaining := int64(16 << 20)
	for remaining > 0 {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			remaining -= int64(n)
			if _, e = w.Write(buf[:n]); e != nil {
				return
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

func jiraMCPToolAllowed(name string) bool {
	return strings.Contains(strings.ToLower(name), "jira") || name == "getAccessibleAtlassianResources" || name == "atlassianUserInfo"
}

func filterJiraToolCatalog(data []byte) ([]byte, error) {
	var doc map[string]json.RawMessage
	if json.Unmarshal(data, &doc) != nil {
		// Streamable HTTP may return a JSON-RPC response inside an SSE event.
		for _, event := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n\n") {
			var payload []string
			for _, line := range strings.Split(event, "\n") {
				if strings.HasPrefix(line, "data:") {
					payload = append(payload, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
				}
			}
			var candidate map[string]json.RawMessage
			if json.Unmarshal([]byte(strings.Join(payload, "\n")), &candidate) == nil && candidate["id"] != nil {
				doc = candidate
				break
			}
		}
	}
	if doc == nil {
		return nil, errors.New("invalid MCP catalog")
	}
	if doc["error"] != nil {
		return json.Marshal(doc)
	}
	var result map[string]json.RawMessage
	if json.Unmarshal(doc["result"], &result) != nil {
		return nil, errors.New("invalid MCP result")
	}
	var tools []map[string]json.RawMessage
	if json.Unmarshal(result["tools"], &tools) != nil {
		return nil, errors.New("invalid MCP tools")
	}
	filtered := make([]map[string]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		var name string
		_ = json.Unmarshal(tool["name"], &name)
		if jiraMCPToolAllowed(name) {
			filtered = append(filtered, tool)
		}
	}
	result["tools"], _ = json.Marshal(filtered)
	doc["result"], _ = json.Marshal(result)
	return json.Marshal(doc)
}

// A late 401 from an old request must not invalidate a newly connected account.
func (h *Handler) invalidateJiraToken(ctx context.Context, user, token string) {
	tx, e := h.TxStarter.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	var sealed []byte
	if e = tx.QueryRow(ctx, `SELECT credentials FROM jira_mcp_connection WHERE user_id=$1 FOR UPDATE`, user).Scan(&sealed); e != nil {
		return
	}
	c, e := h.JiraMCP.open(sealed)
	if e != nil || c.AccessToken != token {
		return
	}
	if _, e = tx.Exec(ctx, `UPDATE jira_mcp_connection SET status='reconnect_required',credentials=NULL WHERE user_id=$1`, user); e == nil {
		_ = tx.Commit(ctx)
	}
}

func (h *Handler) attachJiraMCP(resp *AgentTaskResponse) {
	if h.JiraMCP == nil || resp.Agent == nil || resp.AuthToken == "" {
		return
	}
	// Tasks without a current human requester have no personal Jira principal.
	// Do not mount a gateway that would reject their initialization.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	var eligible bool
	if err := h.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_task_queue t JOIN agent a ON a.id=t.agent_id JOIN member m ON m.workspace_id=a.workspace_id AND m.user_id=t.originator_user_id WHERE t.id=$1)`, resp.ID).Scan(&eligible); err != nil || !eligible {
		return
	}
	resp.Agent.Instructions += "\nFor Jira work use the multica_jira MCP server, which acts as this task's requesting Multica user. Never substitute machine-local Jira credentials, a shared account, or another user's connection. If jira_connection_required is offered, call it only when the work needs Jira: it records a blocker and notifies the requester. Then stop Jira work and tell the requester to Connect Jira on the issue or in Account settings, and select Resume on the issue after connecting. Do not claim changes without a successful tool result.\n"
	headers := map[string]string{"Authorization": "Bearer " + resp.AuthToken}
	if h.cfg.CloudflareClientID != "" {
		headers["CF-Access-Client-Id"] = h.cfg.CloudflareClientID
		headers["CF-Access-Client-Secret"] = h.cfg.CloudflareClientSecret
	}
	overlay, _ := json.Marshal(map[string]any{"mcpServers": map[string]any{"multica_jira": map[string]any{"type": "http", "url": strings.TrimRight(h.cfg.PublicURL, "/") + jiraMCPPath + "/mcp", "headers": headers}}})
	if merged, e := mergeMCPOverlay(resp.Agent.McpConfig, overlay); e == nil {
		resp.Agent.McpConfig = merged
	}
}

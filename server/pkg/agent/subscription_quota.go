package agent

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/multica-ai/multica/server/pkg/quota"
	"io"
	"os"
	"sort"
	"syscall"
	"time"
)

// ReadCodexQuota performs only initialize/account reads. It never creates a thread or turn.
func ReadCodexQuota(ctx context.Context, runtimeCmd Command) (quota.Report, error) {
	r := quota.Report{Provider: "codex", Status: "unknown", Source: "codex_app_server", ObservedAt: time.Now().UTC(), Windows: []quota.Window{}}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := runtimeCmd.exec(ctx, "app-server")
	hideAgentWindow(cmd)
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	in, e := cmd.StdinPipe()
	if e != nil {
		return r, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		in.Close()
		return r, e
	}
	if e = startOwnedProcessTree(cmd, runtimeCmd.logger); e != nil {
		in.Close()
		out.Close()
		return r, e
	}
	defer func() {
		in.Close()
		cancel()
		signalProcessGroup(cmd, syscall.SIGKILL)
		out.Close()
		_ = cmd.Wait()
		releaseProcessGroup(cmd)
	}()
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 1024*1024)
	request := func(id int, method string, params any) (json.RawMessage, error) {
		if e := json.NewEncoder(in).Encode(map[string]any{"id": id, "method": method, "params": params}); e != nil {
			return nil, e
		}
		for scan.Scan() {
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scan.Bytes(), &msg) != nil || msg.ID != id {
				continue
			}
			if len(msg.Error) > 0 && string(msg.Error) != "null" {
				return nil, fmt.Errorf("quota RPC failed")
			}
			return msg.Result, nil
		}
		return nil, fmt.Errorf("quota RPC ended")
	}
	if _, e = request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "multica-quota", "version": "1"}}); e != nil {
		return r, e
	}
	if e = json.NewEncoder(in).Encode(map[string]any{"method": "initialized"}); e != nil {
		return r, e
	}
	account, e := request(2, "account/read", map[string]bool{"refreshToken": false})
	if e != nil {
		return r, e
	}
	var identity struct {
		Account *struct {
			Type      string `json:"type"`
			ID        string `json:"id"`
			AccountID string `json:"accountId"`
		} `json:"account"`
	}
	if json.Unmarshal(account, &identity) != nil || identity.Account == nil {
		return r, fmt.Errorf("account unavailable")
	}
	if identity.Account.Type == "apiKey" {
		r.Status = "not_applicable"
		return r, nil
	}
	raw, e := request(3, "account/rateLimits/read", nil)
	if e != nil {
		return r, e
	}
	r, e = ParseCodexQuota(raw, r.ObservedAt)
	if e != nil {
		return r, e
	}
	id := identity.Account.AccountID
	if id == "" {
		id = identity.Account.ID
	}
	if r.AccountKey == "" && id != "" {
		r.AccountKey = quotaAccountKey("codex", id)
	}
	return r, nil
}
func quotaAccountKey(provider, id string) string {
	h := sha256.Sum256([]byte(provider + ":" + id))
	return hex.EncodeToString(h[:])
}

type codexQuotaWindow struct {
	Used    *float64 `json:"usedPercent"`
	Minutes int64    `json:"windowDurationMins"`
	Reset   int64    `json:"resetsAt"`
}
type codexQuotaBucket struct {
	ID        string            `json:"limitId"`
	AccountID string            `json:"accountId"`
	Primary   *codexQuotaWindow `json:"primary"`
	Secondary *codexQuotaWindow `json:"secondary"`
}

func ParseCodexQuota(raw []byte, at time.Time) (quota.Report, error) {
	r := quota.Report{Provider: "codex", Status: "unknown", Source: "codex_app_server", ObservedAt: at, Windows: []quota.Window{}}
	var v struct {
		AccountID string                      `json:"accountId"`
		Limits    *codexQuotaBucket           `json:"rateLimits"`
		ByID      map[string]codexQuotaBucket `json:"rateLimitsByLimitId"`
	}
	if e := json.Unmarshal(raw, &v); e != nil {
		return r, e
	}
	if len(v.ByID) == 0 && v.Limits != nil {
		key := v.Limits.ID
		if key == "" {
			key = "codex"
		}
		v.ByID = map[string]codexQuotaBucket{key: *v.Limits}
	}
	keys := []string{}
	for k := range v.ByID {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b := v.ByID[key]
		if v.AccountID == "" {
			v.AccountID = b.AccountID
		}
		for i, w := range []*codexQuotaWindow{b.Primary, b.Secondary} {
			if w == nil || w.Minutes <= 0 {
				continue
			}
			r.Windows = append(r.Windows, quota.Window{ID: fmt.Sprintf("%s:%d", key, i), UsedPercent: w.Used, ResetsAt: w.Reset, DurationMinutes: w.Minutes, AppliesAll: key == "codex"})
		}
	}
	if v.AccountID != "" {
		r.AccountKey = quotaAccountKey("codex", v.AccountID)
	}
	if len(r.Windows) > 0 {
		r.Status = "reported"
	}
	return r, r.Validate(at)
}
func ParseClaudeQuota(raw []byte, model string, at time.Time) *quota.Report {
	var v struct {
		Status      string   `json:"status"`
		Type        string   `json:"rateLimitType"`
		Utilization *float64 `json:"utilization"`
		Reset       int64    `json:"resetsAt"`
	}
	if json.Unmarshal(raw, &v) != nil || (v.Status != "allowed" && v.Status != "allowed_warning" && v.Status != "rejected") {
		return nil
	}
	minutes := int64(0)
	all := false
	switch v.Type {
	case "five_hour":
		minutes = 300
		all = true
	case "seven_day":
		minutes = 10080
		all = true
	case "seven_day_opus", "seven_day_sonnet":
		minutes = 10080
	}
	if v.Type == "" {
		v.Type = "reported_limit"
	}
	var used *float64
	if v.Utilization != nil {
		x := *v.Utilization * 100
		used = &x
	}
	r := quota.Report{Provider: "claude", Status: "reported", Source: "claude_stream", ObservedAt: at, Windows: []quota.Window{{ID: v.Type, Model: model, UsedPercent: used, ResetsAt: v.Reset, DurationMinutes: minutes, Exhausted: v.Status == "rejected", AppliesAll: all}}}
	if r.Validate(at) != nil {
		return nil
	}
	return &r
}

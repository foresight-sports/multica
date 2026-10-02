// Package quota contains sanitized subscription-capacity observations, never credentials.
package quota

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"
)

const FreshFor = 10 * time.Minute

type Window struct {
	ID              string   `json:"id"`
	UsedPercent     *float64 `json:"used_percent"`
	ResetsAt        int64    `json:"resets_at"`
	DurationMinutes int64    `json:"duration_minutes"`
	Model           string   `json:"model,omitempty"`
	Exhausted       bool     `json:"exhausted"`
	AppliesAll      bool     `json:"applies_all"`
}
type Report struct {
	Provider   string    `json:"provider"`
	AccountKey string    `json:"account_key,omitempty"`
	Status     string    `json:"status"`
	Source     string    `json:"source"`
	ObservedAt time.Time `json:"observed_at"`
	Windows    []Window  `json:"windows"`
}

func (r Report) Validate(now time.Time) error {
	if (r.Provider == "codex" && r.Source != "codex_app_server") || (r.Provider == "claude" && r.Source != "claude_stream" && r.Source != "claude_usage") {
		return fmt.Errorf("invalid quota source")
	}
	if r.Provider != "codex" && r.Provider != "claude" {
		return fmt.Errorf("unsupported quota provider")
	}
	if r.Status != "reported" && r.Status != "unknown" && r.Status != "not_applicable" {
		return fmt.Errorf("invalid quota status")
	}
	if len(r.AccountKey) > 64 || (r.AccountKey != "" && len(r.AccountKey) != 64) || len(r.Windows) > 32 || len(r.Source) > 40 {
		return fmt.Errorf("invalid quota report")
	}
	for _, c := range r.AccountKey {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return fmt.Errorf("invalid account key")
		}
	}
	if r.ObservedAt.IsZero() || r.ObservedAt.After(now.Add(time.Minute)) || r.ObservedAt.Before(now.Add(-24*time.Hour)) {
		return fmt.Errorf("invalid observation time")
	}
	ids := map[string]bool{}
	for _, w := range r.Windows {
		if w.ID == "" || len(w.ID) > 100 || len(w.Model) > 256 || ids[w.ID] || w.DurationMinutes < 0 || w.ResetsAt < 0 {
			return fmt.Errorf("invalid quota window")
		}
		ids[w.ID] = true
		if w.UsedPercent != nil && (math.IsNaN(*w.UsedPercent) || math.IsInf(*w.UsedPercent, 0) || *w.UsedPercent < 0 || *w.UsedPercent > 10000) {
			return fmt.Errorf("invalid usage percentage")
		}
	}
	return nil
}
func Parse(raw []byte) Report { var r Report; _ = json.Unmarshal(raw, &r); return r }
func (r Report) Fresh(now time.Time) bool {
	return r.Status == "reported" && !r.ObservedAt.IsZero() && now.Sub(r.ObservedAt) >= -time.Minute && now.Sub(r.ObservedAt) <= FreshFor
}

// Passing a reset never implies zero usage: the next window needs a new observation.
func (r Report) Blocks(model string, now time.Time) bool {
	if !r.Fresh(now) {
		return false
	}
	for _, w := range r.Windows {
		if w.ResetsAt > 0 && w.ResetsAt <= now.Unix() {
			continue
		}
		familyMatch := r.Provider == "claude" && r.Source == "claude_usage" &&
			((w.ID == "seven_day_opus" && strings.Contains(strings.ToLower(model), "opus")) ||
				(w.ID == "seven_day_sonnet" && strings.Contains(strings.ToLower(model), "sonnet")))
		if !w.AppliesAll && !familyMatch && (w.Model == "" || w.Model != model) {
			continue
		}
		if w.Exhausted || (w.UsedPercent != nil && *w.UsedPercent >= 100) {
			return true
		}
	}
	return false
}

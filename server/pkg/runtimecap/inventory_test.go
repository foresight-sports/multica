package runtimecap

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestInventoryBoundsFreshnessAndProvider(t *testing.T) {
	now := time.Now()
	r := New("codex")
	r.Status = "reported"
	r.ObservedAt = now
	for i := 0; i < 70; i++ {
		r.Add(Entry{Kind: "skill", Name: "example", Description: strings.Repeat("a", 500), Enabled: Bool(true)})
	}
	if len(r.Entries) != 64 || !r.Truncated || len(r.Entries[0].Description) != 240 || r.Entries[0].Auth != "unknown" {
		t.Fatal("unbounded inventory", r)
	}
	raw, _ := json.Marshal(map[string]any{"runtime_capabilities": r})
	if FromMetadata(raw, "codex", now) == nil || FromMetadata(raw, "claude", now) != nil || FromMetadata(raw, "codex", now.Add(6*time.Minute)) != nil || FromMetadata(raw, "codex", now.Add(-time.Minute)) != nil {
		t.Fatal("provider/freshness boundary")
	}
	if len(r.ForTriage().Entries) != 16 {
		t.Fatal("unbounded routing context")
	}
	r.Entries[0].Kind = "credential"
	if r.Validate("codex") == nil {
		t.Fatal("invalid kind accepted")
	}
}
func TestInventoryRedactsDescriptions(t *testing.T) {
	r := New("codex")
	r.Add(Entry{Kind: "plugin", Name: "example", Description: "Authorization: Bearer private-secret-value"})
	if strings.Contains(r.Entries[0].Description, "private-secret-value") {
		t.Fatal("secret leaked")
	}
}

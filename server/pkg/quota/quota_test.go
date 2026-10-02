package quota

import (
	"testing"
	"time"
)

func TestCapacityFreshnessAndModelScope(t *testing.T) {
	now := time.Now()
	used := 100.0
	r := Report{Status: "reported", ObservedAt: now, Windows: []Window{{UsedPercent: &used, AppliesAll: true, ResetsAt: now.Add(time.Hour).Unix()}}}
	if !r.Blocks("m", now) {
		t.Fatal("exhausted should block")
	}
	if r.Blocks("m", now.Add(11*time.Minute)) {
		t.Fatal("stale must be unknown")
	}
	r.Windows[0].ResetsAt = now.Unix()
	if r.Blocks("m", now) {
		t.Fatal("reset passed must be unknown")
	}
	r.Windows[0].ResetsAt = 0
	r.Windows[0].AppliesAll = false
	r.Windows[0].Model = "other"
	if r.Blocks("m", now) {
		t.Fatal("other model blocked")
	}
	r.Windows[0].Model = "m"
	r.Windows[0].UsedPercent = nil
	if r.Blocks("m", now) {
		t.Fatal("unknown treated as exhausted")
	}
	r.Windows[0].Exhausted = true
	if !r.Blocks("m", now) {
		t.Fatal("explicit rejection ignored")
	}
}

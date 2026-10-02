package execution

import "testing"

func TestSelectionConstraints(t *testing.T) {
	p := Policy{Mode: "automatic", Preference: "balanced", DefaultProfile: "fast", Profiles: []Profile{{ID: "fast", RuntimeID: "r1", Model: "m1", Speed: 5, Quality: 2, Cost: 1}, {ID: "deep", RuntimeID: "r2", Model: "m2", Speed: 1, Quality: 5, Cost: 5, Keywords: []string{"architecture"}}}}
	candidates := []Candidate{{Profile: p.Profiles[0], Eligible: true}, {Profile: p.Profiles[1], Eligible: true}}
	selected, _, err := Select(p, Request{}, candidates, "Review architecture")
	if err != nil || selected.ID != "deep" {
		t.Fatalf("keyword routing: %v %v", selected, err)
	}
	candidates[1].Eligible = false
	selected, _, err = Select(p, Request{}, candidates, "Review architecture")
	if err != nil || selected.ID != "fast" {
		t.Fatal("ineligible profile selected")
	}
	if _, _, err = Select(p, Request{ProfileID: "deep"}, candidates, ""); err == nil {
		t.Fatal("explicit request silently fell back")
	}
	if _, _, err = Select(p, Request{RuntimeID: "r1", Model: "m2"}, candidates, ""); err == nil {
		t.Fatal("unapproved combination accepted")
	}
	p.DefaultProfile = "deep"
	if _, _, err = Select(p, Request{Mode: "default"}, candidates, ""); err == nil {
		t.Fatal("fallback disabled ignored")
	}
	p.AllowFallback = true
	selected, _, err = Select(p, Request{Mode: "default"}, candidates, "")
	if err != nil || selected.ID != "fast" {
		t.Fatal("approved fallback failed")
	}
}
func TestValidatePolicy(t *testing.T) {
	p := Policy{Mode: "automatic", Preference: "balanced", DefaultProfile: "x", Profiles: []Profile{{ID: "x", Name: "Test", Provider: "codex", Model: "model", Quality: 3, Speed: 3, Cost: 3}}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	p.Profiles = append(p.Profiles, p.Profiles[0])
	if p.Validate() == nil {
		t.Fatal("duplicate profiles accepted")
	}
}

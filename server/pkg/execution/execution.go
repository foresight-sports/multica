package execution

import (
	"encoding/json"
	"fmt"
	"github.com/multica-ai/multica/server/pkg/runtimecap"
	"regexp"
	"slices"
	"sort"
	"strings"
)

type Profile struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	RuntimeID     string   `json:"runtime_id,omitempty"`
	Provider      string   `json:"provider"`
	Model         string   `json:"model"`
	ThinkingLevel string   `json:"thinking_level"`
	ServiceTier   string   `json:"service_tier"`
	Purpose       string   `json:"purpose"`
	Keywords      []string `json:"keywords"`
	RequiredOS    string   `json:"required_os"`
	RequiredTools []string `json:"required_tools"`
	Quality       int      `json:"quality"`
	Speed         int      `json:"speed"`
	Cost          int      `json:"cost"`
}
type Policy struct {
	Revision       int64     `json:"revision"`
	Mode           string    `json:"mode"`
	DefaultProfile string    `json:"default_profile"`
	RouterProfile  string    `json:"router_profile"`
	Preference     string    `json:"preference"`
	AllowFallback  bool      `json:"allow_fallback"`
	Profiles       []Profile `json:"profiles"`
}
type Request struct {
	Instruction  string `json:"instruction,omitempty"`
	Mode         string `json:"mode,omitempty"`
	ProfileID    string `json:"profile_id,omitempty"`
	RuntimeID    string `json:"runtime_id,omitempty"`
	Model        string `json:"model,omitempty"`
	FreshSession bool   `json:"fresh_session,omitempty"`
}
type Decision struct {
	ProfileID string `json:"profile_id"`
	RuntimeID string `json:"runtime_id"`
	Model     string `json:"model"`
	Reason    string `json:"reason"`
	At        string `json:"at"`
}
type Selection struct {
	RuntimeCapabilities map[string]*runtimecap.Report `json:"runtime_capabilities,omitempty"`
	Locked              bool                          `json:"locked,omitempty"`
	RoutingFailed       bool                          `json:"routing_failed,omitempty"`
	State               string                        `json:"state"`
	Profile             Profile                       `json:"profile"`
	PolicyRevision      int64                         `json:"policy_revision"`
	Reason              string                        `json:"reason"`
	Explicit            bool                          `json:"explicit"`
	History             []Decision                    `json:"history"`
	Candidates          []Profile                     `json:"candidates,omitempty"`
	Prompt              string                        `json:"prompt,omitempty"`
}
type Candidate struct {
	Capabilities *runtimecap.Report `json:"capabilities,omitempty"`
	Profile      Profile            `json:"profile"`
	Eligible     bool               `json:"eligible"`
	Reason       string             `json:"reason,omitempty"`
}

var toolName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.+-]{0,99}$`)

func ValidToolName(name string) bool { return toolName.MatchString(name) }

func ParsePolicy(raw []byte) Policy       { var p Policy; _ = json.Unmarshal(raw, &p); return p }
func ParseSelection(raw []byte) Selection { var s Selection; _ = json.Unmarshal(raw, &s); return s }
func ParseRequest(raw []byte) Request     { var r Request; _ = json.Unmarshal(raw, &r); return r }
func (p Policy) Validate() error {
	if !slices.Contains([]string{"default", "automatic"}, p.Mode) {
		return fmt.Errorf("mode must be default or automatic")
	}
	if !slices.Contains([]string{"balanced", "speed", "quality", "cost"}, p.Preference) {
		return fmt.Errorf("invalid routing preference")
	}
	if len(p.Profiles) > 24 {
		return fmt.Errorf("at most 24 execution profiles are allowed")
	}
	ids := map[string]bool{}
	for _, v := range p.Profiles {
		if v.ID == "" || len(v.ID) > 64 || ids[v.ID] || strings.TrimSpace(v.Name) == "" || len(v.Name) > 100 || len(v.Purpose) > 2000 {
			return fmt.Errorf("profiles need unique IDs, names and bounded descriptions")
		}
		if len(v.Model) == 0 || len(v.Model) > 256 || strings.TrimSpace(v.Provider) == "" || len(v.Provider) > 100 || v.RuntimeID != "" {
			return fmt.Errorf("profile requires a provider and model; machine selection is automatic")
		}
		for _, value := range append(append([]string{}, v.Keywords...), v.RequiredTools...) {
			if len(value) > 100 {
				return fmt.Errorf("profile keyword or tool is too long")
			}
		}
		ids[v.ID] = true
		for _, tool := range v.RequiredTools {
			if !ValidToolName(tool) {
				return fmt.Errorf("required tools must be command names, not paths")
			}
		}
		if v.Quality < 1 || v.Quality > 5 || v.Speed < 1 || v.Speed > 5 || v.Cost < 1 || v.Cost > 5 {
			return fmt.Errorf("quality, speed and cost ratings must be 1–5")
		}
		if !slices.Contains([]string{"", "windows", "linux", "darwin"}, v.RequiredOS) {
			return fmt.Errorf("unsupported operating system requirement")
		}
		if len(v.Keywords) > 30 || len(v.RequiredTools) > 20 {
			return fmt.Errorf("too many profile requirements")
		}
	}
	if len(p.Profiles) > 0 && !ids[p.DefaultProfile] {
		return fmt.Errorf("choose a default profile")
	}
	if p.RouterProfile != "" && !ids[p.RouterProfile] {
		return fmt.Errorf("choose an approved routing profile")
	}
	return nil
}

// Select never widens the eligible set. Explicit requests are exact constraints.
func Select(p Policy, req Request, candidates []Candidate, prompt string) (Profile, string, error) {
	valid := []Profile{}
	for _, c := range candidates {
		if c.Eligible {
			valid = append(valid, c.Profile)
		}
	}
	if req.ProfileID != "" || req.RuntimeID != "" || req.Model != "" {
		for _, v := range valid {
			if (req.ProfileID == "" || req.ProfileID == v.ID) && (req.RuntimeID == "" || req.RuntimeID == v.RuntimeID) && (req.Model == "" || req.Model == v.Model) {
				return v, "Explicit task selection", nil
			}
		}
		return Profile{}, "", fmt.Errorf("the requested execution profile is unavailable or not approved")
	}
	mode := p.Mode
	if req.Mode != "" {
		mode = req.Mode
	}
	if mode == "default" {
		for _, v := range valid {
			if v.ID == p.DefaultProfile {
				return v, "Agent default profile", nil
			}
		}
		if !p.AllowFallback {
			return Profile{}, "", fmt.Errorf("default profile unavailable; fallback is disabled")
		}
	}
	if len(valid) == 0 {
		return Profile{}, "", fmt.Errorf("no eligible runtime/model combination is online")
	}
	text := strings.ToLower(prompt)
	score := func(v Profile) int {
		n := v.Quality + v.Speed - v.Cost
		switch p.Preference {
		case "quality":
			n = 4*v.Quality + v.Speed - v.Cost
		case "speed":
			n = 4*v.Speed + v.Quality - v.Cost
		case "cost":
			n = 10 - 4*v.Cost + v.Quality + v.Speed
		}
		for _, k := range v.Keywords {
			if k != "" && strings.Contains(text, strings.ToLower(k)) {
				n += 20
			}
		}
		if v.ID == p.DefaultProfile {
			n++
		}
		return n
	}
	sort.SliceStable(valid, func(i, j int) bool { return score(valid[i]) > score(valid[j]) })
	reason := "Automatic selection using task keywords and " + p.Preference + " preference"
	if mode == "default" {
		reason = "Default runtime unavailable; approved fallback selected"
	}
	return valid[0], reason, nil
}

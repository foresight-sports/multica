// Package workflow defines durable, workspace-scoped installation and work handoffs.
package workflow

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

type Data struct {
	Tool         string       `json:"tool,omitempty"`
	Version      string       `json:"version,omitempty"`
	Source       string       `json:"source,omitempty"`
	Scope        string       `json:"scope,omitempty"`
	Effects      string       `json:"effects,omitempty"`
	Reason       string       `json:"reason,omitempty"`
	Provider     string       `json:"provider,omitempty"`
	Machine      string       `json:"machine,omitempty"`
	Verification string       `json:"verification,omitempty"`
	Repository   string       `json:"repository,omitempty"`
	Branch       string       `json:"branch,omitempty"`
	Commit       string       `json:"commit,omitempty"`
	PRURL        string       `json:"pr_url,omitempty"`
	Validation   string       `json:"validation,omitempty"`
	NextStep     string       `json:"next_step,omitempty"`
	Owner        string       `json:"owner,omitempty"`
	Text         string       `json:"text,omitempty"`
	AuthorType   string       `json:"author_type,omitempty"`
	Dependencies []Dependency `json:"dependencies,omitempty"`
}
type Dependency struct {
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

var sha = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)
var repo = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func ValidBranch(s string) bool {
	if s == "@" || strings.HasSuffix(s, ".") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".lock") {
			return false
		}
	}
	for _, c := range s {
		if c < 32 || c == 127 {
			return false
		}
	}
	return len(s) > 0 && len(s) <= 240 && !strings.HasPrefix(s, "-") && !strings.HasPrefix(s, "/") && !strings.HasSuffix(s, "/") && !strings.ContainsAny(s, " ~^:?*[\\\t\r\n") && !strings.Contains(s, "..") && !strings.Contains(s, "@{") && !strings.Contains(s, "//") && !strings.HasSuffix(s, ".lock")
}
func (d Data) Validate(kind string) error {
	for _, s := range []string{d.Tool, d.Version, d.Source, d.Scope, d.Effects, d.Reason, d.Verification, d.Repository, d.Branch, d.Commit, d.PRURL, d.Validation, d.NextStep, d.Owner, d.Text} {
		if len(s) > 4000 {
			return fmt.Errorf("workflow fields must be at most 4000 bytes")
		}
	}
	switch kind {
	case "installation":
		u, e := url.Parse(d.Source)
		if strings.TrimSpace(d.Tool) == "" || d.Version == "" || e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || (d.Scope != "user" && d.Scope != "system") || strings.TrimSpace(d.Reason) == "" {
			return fmt.Errorf("installation requires tool, version, HTTPS source, user/system scope and reason")
		}
	case "checkpoint":
		if d.PRURL != "" {
			u, e := url.Parse(d.PRURL)
			if e != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil {
				return fmt.Errorf("PR URL must be an HTTPS GitHub URL")
			}
		}
		if !repo.MatchString(d.Repository) || !ValidBranch(d.Branch) || (d.Commit != "" && !sha.MatchString(d.Commit)) {
			return fmt.Errorf("checkpoint requires owner/repository, a work branch and a full commit SHA when saved")
		}
		if len(d.Dependencies) > 20 {
			return fmt.Errorf("at most 20 dependencies are allowed")
		}
		for _, dep := range d.Dependencies {
			if !repo.MatchString(dep.Repository) || !sha.MatchString(dep.Commit) {
				return fmt.Errorf("dependencies require repository and full commit SHA")
			}
		}
	case "decision", "blocker":
		if strings.TrimSpace(d.Text) == "" {
			return fmt.Errorf("text is required")
		}
	default:
		return fmt.Errorf("unknown work record kind")
	}
	return nil
}

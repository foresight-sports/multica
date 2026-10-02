package workspacerepo

import (
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"
)

type Configuration struct {
	Repository string `json:"repository"`
	Folder     string `json:"folder"`
	Root       string `json:"root"`
	Mode       string `json:"mode"`
}

var segment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
var drive = regexp.MustCompile(`^[A-Za-z]:/`)

func (c *Configuration) Validate(scope string) error {
	c.Repository = strings.TrimSpace(c.Repository)
	c.Folder = strings.TrimSpace(c.Folder)
	c.Root = strings.TrimSpace(c.Root)
	if scope == "machine" {
		if c.Repository != "" || c.Folder != "" || c.Mode != "" {
			return fmt.Errorf("machine configuration only accepts a repositories folder")
		}
		if c.Root == "" {
			return nil
		}
		p := strings.ReplaceAll(c.Root, `\`, "/")
		if !(strings.HasPrefix(p, "/") || drive.MatchString(p)) || strings.ContainsAny(p, "\x00\r\n") || len(p) > 2048 {
			return fmt.Errorf("repositories folder must be an absolute path")
		}
		for _, part := range strings.Split(p, "/") {
			if part == ".." {
				return fmt.Errorf("repositories folder cannot contain parent traversal")
			}
		}
		if strings.Trim(p, "/") == "" || (drive.MatchString(p) && len(strings.TrimRight(p, "/")) == 2) {
			return fmt.Errorf("choose a repositories folder, not a drive root")
		}
		c.Root = strings.TrimRight(p, "/")
		return nil
	}
	if c.Root != "" {
		return fmt.Errorf("set the repositories folder on the machine")
	}
	if c.Repository == "" && c.Folder == "" {
		c.Mode = ""
		return nil
	}
	pieces := strings.Split(c.Repository, "/")
	if c.Repository != "" {
		if len(pieces) != 2 || !segment.MatchString(pieces[0]) || !segment.MatchString(pieces[1]) || len(c.Repository) > 200 {
			return fmt.Errorf("repository must be GitHub owner/repository")
		}
		if c.Folder == "" {
			c.Folder = pieces[1]
		}
	}
	if !segment.MatchString(c.Folder) || strings.HasSuffix(c.Folder, ".") || len(c.Folder) > 200 {
		return fmt.Errorf("local folder must be a single folder name")
	}
	if c.Mode == "" {
		c.Mode = "worktree"
		if c.Repository == "" {
			c.Mode = "in_place"
		}
	}
	if c.Mode != "worktree" && c.Mode != "in_place" {
		return fmt.Errorf("execution mode must be worktree or in_place")
	}
	return nil
}
func Join(root, folder string) string {
	return strings.TrimRight(strings.ReplaceAll(root, `\`, "/"), "/") + "/" + folder
}

// GitHubIdentity compares SSH and HTTPS origins without exposing embedded credentials.
func GitHubIdentity(remote string) string {
	remote = strings.TrimSpace(remote)
	if strings.HasPrefix(remote, "git@github.com:") {
		remote = "https://github.com/" + strings.TrimPrefix(remote, "git@github.com:")
	}
	u, err := url.Parse(remote)
	if err != nil || !strings.EqualFold(u.Hostname(), "github.com") || (u.Scheme != "https" && u.Scheme != "ssh") {
		return ""
	}
	p := strings.TrimSuffix(strings.Trim(u.Path, "/"), ".git")
	if path.Clean(p) != p {
		return ""
	}
	c := Configuration{Repository: p}
	if c.Validate("workspace") != nil {
		return ""
	}
	return strings.ToLower(p)
}

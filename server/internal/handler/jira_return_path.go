package handler

import (
	"net/url"
	"strings"
)

// Keep the initiating page with the encrypted OAuth state, never in an
// untrusted callback parameter. Only this Multica origin may be a destination.
func jiraReturnPath(publicURL, candidate string) string {
	if candidate == "" || len(candidate) > 4096 || strings.ContainsAny(candidate, "\\\r\n\x00") {
		return "/"
	}
	base, err := url.Parse(publicURL)
	if err != nil {
		return "/"
	}
	target, err := url.Parse(candidate)
	if err != nil || target.User != nil {
		return "/"
	}
	if target.IsAbs() {
		if target.Scheme != base.Scheme || target.Host != base.Host {
			return "/"
		}
	} else if target.Host != "" || !strings.HasPrefix(candidate, "/") || strings.HasPrefix(candidate, "//") {
		return "/"
	}
	// Check the decoded path too: browsers normalize encoded slashes/backslashes.
	if strings.HasPrefix(target.Path, "//") || strings.ContainsAny(target.Path, "\\\r\n\x00") || strings.HasPrefix(target.Path, "/api/") {
		return "/"
	}
	result := target.EscapedPath()
	if result == "" {
		result = "/"
	}
	if target.RawQuery != "" {
		result += "?" + target.RawQuery
	}
	if target.Fragment != "" {
		result += "#" + target.EscapedFragment()
	}
	return result
}

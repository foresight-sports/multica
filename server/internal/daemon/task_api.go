package daemon

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/multica-ai/multica/server/internal/accessclient"
)

// This gateway authenticates a currently active task, never substitutes the
// daemon owner's API token, and injects only the daemon's Cloudflare headers.
func (d *Daemon) taskAPIHandler() http.Handler {
	target, err := url.Parse(d.cfg.ServerBaseURL)
	if err != nil || target.Host == "" || target.User != nil || (target.Scheme != "https" && target.Scheme != "http") {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "invalid upstream", 503) })
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		task, auth := d.activeRepoCheckoutTask(r)
		if auth != repoCheckoutAuthOK {
			http.Error(w, "active task authentication required", 401)
			return
		}
		if !strings.HasPrefix(r.URL.Path, "/task-api/api/") {
			http.NotFound(w, r)
			return
		}
		proxy := &httputil.ReverseProxy{
			Transport: accessclient.Transport(nil),
			Rewrite: func(p *httputil.ProxyRequest) {
				p.Out.URL.Path = strings.TrimPrefix(p.Out.URL.Path, "/task-api")
				if p.Out.URL.RawPath != "" {
					p.Out.URL.RawPath = strings.TrimPrefix(p.Out.URL.RawPath, "/task-api")
				}
				p.SetURL(target)
				p.Out.Header.Set("X-Workspace-ID", task.WorkspaceID)
				p.Out.Header.Del("Cookie")
				p.Out.Header.Del("CF-Access-Client-Id")
				p.Out.Header.Del("CF-Access-Client-Secret")
			},
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				http.Error(w, "task API gateway unavailable", 502)
			},
		}
		proxy.ServeHTTP(w, r)
	})
}

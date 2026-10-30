package accessclient

import (
	"net/http"
	"net/url"
	"os"
	"strings"
)

// TaskTransport keeps Cloudflare host credentials in the daemon. The child
// retains only its existing task token, and canonical API URLs remain intact.
func TaskTransport(origin string) http.RoundTripper {
	upstream, err := url.Parse(origin)
	if err != nil {
		return Transport(nil)
	}
	proxy, err := url.Parse(os.Getenv("MULTICA_TASK_API_PROXY"))
	if err != nil || proxy.Scheme != "http" || proxy.Hostname() != "127.0.0.1" || proxy.Port() == "" || proxy.User != nil || proxy.Path != "/task-api" || proxy.RawQuery != "" || proxy.Fragment != "" {
		return Transport(nil)
	}
	return &taskTransport{origin: upstream, proxy: proxy, direct: Transport(nil), local: http.DefaultTransport}
}

type taskTransport struct {
	origin, proxy *url.URL
	direct, local http.RoundTripper
}

func (t *taskTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != t.origin.Scheme || !strings.EqualFold(req.URL.Host, t.origin.Host) || req.URL.User != nil || !strings.HasPrefix(req.URL.Path, "/api/") {
		return t.direct.RoundTrip(req)
	}
	clone := req.Clone(req.Context())
	u := *req.URL
	u.Scheme, u.Host = t.proxy.Scheme, t.proxy.Host
	u.Path = t.proxy.Path + u.Path
	if u.RawPath != "" {
		u.RawPath = t.proxy.Path + u.RawPath
	}
	clone.URL, clone.Host = &u, t.proxy.Host
	clone.Header.Del("CF-Access-Client-Id")
	clone.Header.Del("CF-Access-Client-Secret")
	return t.local.RoundTrip(clone)
}

package accessclient

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTaskTransportDoesNotReadHostCredentials(t *testing.T) {
	config(t, "invalid host credentials")
	local := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RequestURI() != "/task-api/api/issues/test?limit=2" {
			t.Errorf("unexpected target: %s", r.URL)
		}
		if r.Header.Get("Authorization") != "Bearer mat_test" {
			t.Error("lost task token")
		}
		if r.Header.Get("CF-Access-Client-Secret") != "" {
			t.Error("forwarded host credential")
		}
		w.Write([]byte("ok"))
	}))
	defer local.Close()
	t.Setenv("MULTICA_TASK_API_PROXY", local.URL+"/task-api")
	req, _ := http.NewRequest("GET", "https://multica.example/api/issues/test?limit=2", nil)
	req.Header.Set("Authorization", "Bearer mat_test")
	req.Header.Set("CF-Access-Client-Secret", "must-not-forward")
	resp, err := TaskTransport("https://multica.example").RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("body = %q", body)
	}
	if req.URL.Host != "multica.example" || req.Header.Get("CF-Access-Client-Secret") != "must-not-forward" {
		t.Fatal("mutated caller request")
	}
}

func TestTaskTransportScope(t *testing.T) {
	t.Setenv("MULTICA_TASK_API_PROXY", "http://127.0.0.1:9999/task-api")
	transport := TaskTransport("https://multica.example").(*taskTransport)
	directCalls := 0
	transport.direct = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		directCalls++
		return &http.Response{StatusCode: 204}, nil
	})
	transport.local = roundTripFunc(func(r *http.Request) (*http.Response, error) { t.Fatal("off-origin request proxied"); return nil, nil })
	for _, target := range []string{"https://other.example/api/issues", "http://multica.example/api/issues", "https://multica.example/files/test"} {
		req, _ := http.NewRequest("GET", target, nil)
		if _, err := transport.RoundTrip(req); err != nil {
			t.Fatal(err)
		}
	}
	if directCalls != 3 {
		t.Fatalf("direct calls = %d", directCalls)
	}
}

func TestTaskTransportRejectsNonLoopbackProxy(t *testing.T) {
	for _, proxy := range []string{"", "http://evil.example:80/task-api", "http://user@127.0.0.1:80/task-api", "http://127.0.0.1:80/other", "http://127.0.0.1:80/task-api?x=1"} {
		t.Setenv("MULTICA_TASK_API_PROXY", proxy)
		if _, ok := TaskTransport("https://multica.example").(*taskTransport); ok {
			t.Errorf("accepted %q", proxy)
		}
	}
}

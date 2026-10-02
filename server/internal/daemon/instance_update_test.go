package daemon

import (
	"context"
	"encoding/json"
	"github.com/multica-ai/multica/server/pkg/daemonrelease"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestInstanceUpdateDownload(t *testing.T) {
	for _, mode := range []string{"ok", "changed", "oversized", "redirect", "truncated"} {
		t.Run(mode, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing authorization")
				}
				if r.URL.Path == "/api/daemon/release" {
					size := int64(4)
					version := "0.4.44-foresight.11"
					if mode == "oversized" {
						size = daemonrelease.MaxBinarySize + 1
					}
					if mode == "changed" {
						version = "0.4.44-foresight.12"
					}
					json.NewEncoder(w).Encode(daemonrelease.Manifest{Version: version, Assets: map[string]daemonrelease.Asset{runtime.GOOS + "-" + runtime.GOARCH: {Size: size, SHA256: "checksum"}}})
					return
				}
				if mode == "redirect" {
					http.Redirect(w, r, "/elsewhere", 302)
					return
				}
				if mode == "truncated" {
					w.Write([]byte("a"))
					return
				}
				w.Write([]byte("data"))
			}))
			defer s.Close()
			d := &Daemon{client: &Client{baseURL: s.URL, token: "test-token", client: s.Client()}, cfg: Config{CLIVersion: "0.4.44-foresight.10"}}
			data, _, err := d.downloadInstanceUpdate(context.Background(), "0.4.44-foresight.11")
			if mode == "ok" {
				if err != nil || string(data) != "data" {
					t.Fatalf("%s %v", data, err)
				}
			} else if err == nil {
				t.Fatal("invalid download accepted")
			}
		})
	}
}

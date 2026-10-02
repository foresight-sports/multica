package handler

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/pkg/daemonrelease"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func (h *Handler) daemonRelease() (*daemonrelease.Manifest, error) {
	if h.cfg.DaemonReleaseDir == "" {
		return nil, fmt.Errorf("instance updates are not configured")
	}
	raw, e := os.ReadFile(filepath.Join(h.cfg.DaemonReleaseDir, "manifest.json"))
	if e != nil {
		return nil, e
	}
	var m daemonrelease.Manifest
	if json.Unmarshal(raw, &m) != nil {
		return nil, fmt.Errorf("invalid release manifest")
	}
	if _, e := daemonrelease.Parts(m.Version); e != nil {
		return nil, e
	}
	if len(m.Assets) == 0 {
		return nil, fmt.Errorf("release has no artifacts")
	}
	for _, a := range m.Assets {
		sum, e := hex.DecodeString(a.SHA256)
		if e != nil || len(sum) != 32 || a.Size <= 0 || a.Size > daemonrelease.MaxBinarySize {
			return nil, fmt.Errorf("invalid release artifact")
		}
	}
	return &m, nil
}
func (h *Handler) GetDaemonRelease(w http.ResponseWriter, r *http.Request) {
	m, e := h.daemonRelease()
	if e != nil {
		writeError(w, 503, "instance release unavailable")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, m)
}
func (h *Handler) DownloadDaemonRelease(w http.ResponseWriter, r *http.Request) {
	m, e := h.daemonRelease()
	if e != nil || m.Version != chi.URLParam(r, "version") {
		writeError(w, 404, "release unavailable")
		return
	}
	osName, arch := chi.URLParam(r, "os"), chi.URLParam(r, "arch")
	name := daemonrelease.AssetName(osName, arch)
	a, ok := m.Assets[osName+"-"+arch]
	if !ok || name == "" {
		writeError(w, 404, "platform unavailable")
		return
	}
	f, e := os.Open(filepath.Join(h.cfg.DaemonReleaseDir, m.Version, name))
	if e != nil {
		writeError(w, 503, "release artifact unavailable")
		return
	}
	defer f.Close()
	stat, e := f.Stat()
	if e != nil || !stat.Mode().IsRegular() || stat.Size() != a.Size {
		writeError(w, 503, "release artifact invalid")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, name, stat.ModTime(), f)
}

type runtimeUpdateStatus struct {
	Channel         string `json:"channel"`
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	RemoteSupported bool   `json:"remote_supported"`
	Online          bool   `json:"online"`
	CanUpdate       bool   `json:"can_update"`
	Reason          string `json:"reason"`
}

func (h *Handler) instanceUpdateStatus(rt db.AgentRuntime, canEdit bool) runtimeUpdateStatus {
	var meta struct {
		Version    string `json:"cli_version"`
		OS         string `json:"os"`
		Arch       string `json:"arch"`
		Protocol   int    `json:"instance_update_version"`
		LaunchedBy string `json:"launched_by"`
	}
	_ = json.Unmarshal(rt.Metadata, &meta)
	s := runtimeUpdateStatus{Channel: "instance", CurrentVersion: meta.Version, Online: rt.Status == "online" && rt.LastSeenAt.Valid && time.Since(rt.LastSeenAt.Time) < 90*time.Second, RemoteSupported: meta.Protocol == 1 && (meta.Version == "0.4.44-foresight.10" || daemonrelease.Newer(meta.Version, "0.4.44-foresight.10"))}
	m, e := h.daemonRelease()
	if e != nil {
		s.Reason = "release_unavailable"
		return s
	}
	s.LatestVersion = m.Version
	s.UpdateAvailable = daemonrelease.Newer(m.Version, meta.Version)
	switch {
	case meta.LaunchedBy == "desktop":
		s.Reason = "managed_by_desktop"
	case !s.RemoteSupported:
		s.Reason = "bootstrap_required"
	case meta.Version == m.Version:
		s.Reason = "current"
	case !s.UpdateAvailable:
		s.Reason = "unrecognized_or_newer"
	case !canEdit:
		s.Reason = "read_only"
	case rt.Status != "online" || !rt.LastSeenAt.Valid || time.Since(rt.LastSeenAt.Time) > 90*time.Second:
		s.Reason = "offline"
	default:
		if _, ok := m.Assets[meta.OS+"-"+meta.Arch]; !ok {
			s.Reason = "platform_unavailable"
		} else {
			s.CanUpdate = true
		}
	}
	return s
}
func (h *Handler) GetRuntimeUpdateStatus(w http.ResponseWriter, r *http.Request) {
	rt, member, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if h.cfg.DaemonReleaseDir == "" {
		writeJSON(w, 200, runtimeUpdateStatus{Channel: "upstream"})
		return
	}
	writeJSON(w, 200, h.instanceUpdateStatus(rt, canEditRuntime(member, rt)))
}

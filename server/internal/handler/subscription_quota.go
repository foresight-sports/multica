package handler

import (
	"encoding/json"
	"github.com/go-chi/chi/v5"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/quota"
	"net/http"
	"time"
)

func (h *Handler) ReportSubscriptionQuota(w http.ResponseWriter, r *http.Request) {
	rt, ok := h.requireDaemonRuntimeAccess(w, r, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	var report quota.Report
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768)).Decode(&report) != nil || report.Validate(time.Now()) != nil || report.Provider != rt.Provider || rt.ProfileID.Valid {
		writeError(w, 400, "invalid subscription quota report")
		return
	}
	if report.Windows == nil {
		report.Windows = []quota.Window{}
	}
	data, _ := json.Marshal(report)
	if h.Queries.SetSubscriptionQuota(r.Context(), db.SetSubscriptionQuotaParams{ID: rt.ID, Report: data}) != nil {
		writeError(w, 500, "could not store quota report")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (h *Handler) GetSubscriptionQuota(w http.ResponseWriter, r *http.Request) {
	rt, _, ok := h.requireRuntimeReadAccess(w, r, obsmetrics.RuntimeLookupSourceRuntimeAPI, chi.URLParam(r, "runtimeId"))
	if !ok {
		return
	}
	var metadata struct {
		Report quota.Report `json:"subscription_quota"`
	}
	_ = json.Unmarshal(rt.Metadata, &metadata)
	report := metadata.Report
	if raw, err := h.Queries.GetSubscriptionQuota(r.Context(), rt.ID); err == nil {
		report = quota.Parse(raw)
	}
	report.AccountKey = ""
	if report.Status == "" {
		report.Status = "unknown"
	}
	if report.Windows == nil {
		report.Windows = []quota.Window{}
	}
	if report.Status == "reported" && !report.Fresh(time.Now()) {
		report.Status = "stale"
	}
	writeJSON(w, 200, report)
}

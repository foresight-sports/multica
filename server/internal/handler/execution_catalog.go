package handler

import (
	"encoding/json"
	"time"
)

// Reported choices authorize configuration, not admission by the provider.
// Unlike the discovery cache, these include the static choices the daemon
// explicitly returned to a picker. Provider execution checks remain authoritative.
func reportedExecutionCatalog(metadata []byte, now time.Time) *ModelCatalogSnapshot {
	var m struct {
		Models     []ModelEntry `json:"execution_models"`
		ObservedAt time.Time    `json:"execution_catalog_observed_at"`
		Source     string       `json:"execution_catalog_source"`
	}
	if json.Unmarshal(metadata, &m) != nil || m.ObservedAt.IsZero() || now.Sub(m.ObservedAt) > modelCatalogServeWindow || m.ObservedAt.After(now.Add(time.Minute)) || (m.Source != "discovered" && m.Source != "fallback") || len(m.Models) == 0 {
		return nil
	}
	return &ModelCatalogSnapshot{Models: m.Models, Supported: true, StoredAt: m.ObservedAt}
}

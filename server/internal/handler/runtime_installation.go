package handler

import (
	_ "embed"
	"net/http"
	"strings"
)

//go:embed runtime_install_command.ps1
var runtimeInstallCommand string

func quotePowerShell(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

// GetRuntimeInstallation discloses the shared service credential only to human
// users who can register runtimes. Never place this response in public config.
func (h *Handler) GetRuntimeInstallation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store, private")
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
	if err != nil || !h.runtimeRegistrationAllowed(r.Context(), user.Email) {
		writeError(w, http.StatusForbidden, "you are not allowed to register runtimes")
		return
	}
	id := strings.TrimSpace(h.cfg.CloudflareClientID)
	secret := strings.TrimSpace(h.cfg.CloudflareClientSecret)
	if id == "" || secret == "" || strings.ContainsAny(id+secret, "\r\n") {
		writeError(w, http.StatusServiceUnavailable, "the administrator must configure the Cloudflare service token")
		return
	}
	command := strings.NewReplacer("{{CLIENT_ID}}", quotePowerShell(id), "{{CLIENT_SECRET}}", quotePowerShell(secret)).Replace(runtimeInstallCommand)
	writeJSON(w, http.StatusOK, map[string]string{"command": command})
}

package handler

import (
	"encoding/json"
	"net/http"
)

type instanceMembership struct {
	WorkspaceID string `json:"workspace_id"`
	Role        string `json:"role"`
}

type instanceMember struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Email       string               `json:"email"`
	Workspaces  []string             `json:"workspaces"`
	Memberships []instanceMembership `json:"memberships"`
}

// This directory includes accounts without a workspace. Workspace membership
// alone must never disclose the instance's account inventory.
func (h *Handler) ListInstanceMembers(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "human administrator required")
		return
	}
	if _, ok := h.requirePermissionManager(w, r); !ok {
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT u.id::text,u.name,u.email,
 COALESCE(array_agg(w.name || ' (' || m.role || ')' ORDER BY w.name) FILTER (WHERE w.id IS NOT NULL),ARRAY[]::text[]),
 COALESCE(jsonb_agg(jsonb_build_object('workspace_id',w.id,'role',m.role)) FILTER (WHERE w.id IS NOT NULL),'[]'::jsonb)
 FROM "user" u LEFT JOIN member m ON m.user_id=u.id LEFT JOIN workspace w ON w.id=m.workspace_id
 GROUP BY u.id ORDER BY lower(u.email),u.id LIMIT 1001`)
	if err != nil {
		writeError(w, 500, "could not load instance members")
		return
	}
	defer rows.Close()
	members := []instanceMember{}
	for rows.Next() {
		var member instanceMember
		var memberships []byte
		if err := rows.Scan(&member.ID, &member.Name, &member.Email, &member.Workspaces, &memberships); err != nil {
			writeError(w, 500, "could not load instance members")
			return
		}
		if err := json.Unmarshal(memberships, &member.Memberships); err != nil {
			writeError(w, 500, "could not load memberships")
			return
		}
		members = append(members, member)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not load instance members")
		return
	}
	truncated := len(members) > 1000
	if truncated {
		members = members[:1000]
	}
	rows.Close()
	workspaceRows, err := h.DB.Query(r.Context(), `SELECT id::text,name,slug FROM workspace ORDER BY name,id LIMIT 1001`)
	if err != nil {
		writeError(w, 500, "could not load workspaces")
		return
	}
	defer workspaceRows.Close()
	workspaces := []map[string]string{}
	for workspaceRows.Next() {
		var id, name, slug string
		if err := workspaceRows.Scan(&id, &name, &slug); err != nil {
			writeError(w, 500, "could not load workspaces")
			return
		}
		workspaces = append(workspaces, map[string]string{"id": id, "name": name, "slug": slug})
	}
	if workspaceRows.Err() != nil {
		writeError(w, 500, "could not load workspaces")
		return
	}
	if len(workspaces) > 1000 {
		workspaces = workspaces[:1000]
		truncated = true
	}
	w.Header().Set("Cache-Control", "no-store, private")
	writeJSON(w, 200, map[string]any{"members": members, "truncated": truncated, "workspaces": workspaces})
}

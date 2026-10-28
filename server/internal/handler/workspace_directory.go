package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

type workspaceDirectoryEntry struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Role          string `json:"role"`
	RequestStatus string `json:"request_status"`
}

// Discovery deliberately excludes settings, context, repositories and members.
func (h *Handler) WorkspaceDirectory(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "human account required")
		return
	}
	search := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(search) > 200 {
		writeError(w, 400, "search is too long")
		return
	}
	rows, err := h.DB.Query(r.Context(), `SELECT w.id::text,w.name,w.slug,COALESCE(m.role,''),COALESCE(a.status,'')
 FROM workspace w LEFT JOIN member m ON m.workspace_id=w.id AND m.user_id=$1
 LEFT JOIN workspace_access_request a ON a.workspace_id=w.id AND a.user_id=$1
 WHERE strpos(lower(w.name || ' ' || w.slug),lower($2))>0 ORDER BY lower(w.name),w.id LIMIT 201`, uid, search)
	if err != nil {
		writeError(w, 500, "could not load workspace directory")
		return
	}
	defer rows.Close()
	entries := []workspaceDirectoryEntry{}
	for rows.Next() {
		var e workspaceDirectoryEntry
		if rows.Scan(&e.ID, &e.Name, &e.Slug, &e.Role, &e.RequestStatus) != nil {
			writeError(w, 500, "could not load workspace directory")
			return
		}
		entries = append(entries, e)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not load workspace directory")
		return
	}
	truncated := len(entries) > 200
	if truncated {
		entries = entries[:200]
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(uid))
	if err != nil {
		writeError(w, 500, "could not load account")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"workspaces": entries, "truncated": truncated, "can_create": !h.cfg.DisableWorkspaceCreation && h.actionAllowed(r.Context(), user.Email, permissionCreateWorkspace)})
}

func (h *Handler) RequestWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "human account required")
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not save request")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if err = q.LockSubscriberWrites(r.Context(), db.LockSubscriberWritesParams{WorkspaceID: ws, UserID: parseUUID(uid)}); err == nil {
		err = lockWorkspaceMembership(r.Context(), tx, ws)
	}
	if err != nil {
		writeError(w, 404, "workspace not found")
		return
	}
	if r.Method == http.MethodDelete {
		_, err = tx.Exec(r.Context(), `UPDATE workspace_access_request SET status='cancelled',updated_at=now() WHERE workspace_id=$1 AND user_id=$2 AND status='pending'`, ws, uid)
	} else {
		var member bool
		err = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM member WHERE workspace_id=$1 AND user_id=$2)`, ws, uid).Scan(&member)
		if err != nil {
			writeError(w, 500, "could not check membership")
			return
		}
		if member {
			writeError(w, 409, "you already belong to this workspace")
			return
		}
		_, err = tx.Exec(r.Context(), `INSERT INTO workspace_access_request(workspace_id,user_id) VALUES($1,$2)
   ON CONFLICT(workspace_id,user_id) DO UPDATE SET status='pending',requested_at=now(),updated_at=now(),decided_by=NULL
   WHERE workspace_access_request.status<>'pending'`, ws, uid)
	}
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, 500, "could not save request")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type workspaceAccessReview struct {
	WorkspaceID   string    `json:"workspace_id"`
	WorkspaceName string    `json:"workspace_name"`
	UserID        string    `json:"user_id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	RequestedAt   time.Time `json:"requested_at"`
}

func (h *Handler) ListWorkspaceAccessRequests(w http.ResponseWriter, r *http.Request) {
	uid, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "human account required")
		return
	}
	user, err := h.Queries.GetUser(r.Context(), parseUUID(uid))
	if err != nil {
		writeError(w, 403, "account unavailable")
		return
	}
	manager := emailAllowedForPermission(user.Email, h.cfg.PermissionManagerEmails)
	rows, err := h.DB.Query(r.Context(), `SELECT w.id::text,w.name,u.id::text,u.name,u.email,a.requested_at
 FROM workspace_access_request a JOIN workspace w ON w.id=a.workspace_id JOIN "user" u ON u.id=a.user_id
 WHERE a.status='pending' AND NOT EXISTS(SELECT 1 FROM member m WHERE m.workspace_id=w.id AND m.user_id=u.id)
 AND ($2 OR EXISTS(SELECT 1 FROM member m WHERE m.workspace_id=w.id AND m.user_id=$1 AND m.role IN ('owner','admin')))
 ORDER BY a.requested_at,w.id,u.id LIMIT 201`, uid, manager)
	if err != nil {
		writeError(w, 500, "could not load access requests")
		return
	}
	defer rows.Close()
	entries := []workspaceAccessReview{}
	for rows.Next() {
		var e workspaceAccessReview
		if rows.Scan(&e.WorkspaceID, &e.WorkspaceName, &e.UserID, &e.Name, &e.Email, &e.RequestedAt) != nil {
			writeError(w, 500, "could not load access requests")
			return
		}
		entries = append(entries, e)
	}
	if rows.Err() != nil {
		writeError(w, 500, "could not load access requests")
		return
	}
	truncated := len(entries) > 200
	if truncated {
		entries = entries[:200]
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, map[string]any{"requests": entries, "truncated": truncated})
}

func (h *Handler) ReviewWorkspaceAccess(w http.ResponseWriter, r *http.Request) {
	actor, ok := requireUserID(w, r)
	if !ok {
		return
	}
	if isMachineCredentialActor(r) {
		writeError(w, 403, "human administrator required")
		return
	}
	ws, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workspace id")
	if !ok {
		return
	}
	target, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "userId"), "user id")
	if !ok {
		return
	}
	var req struct {
		Decision string `json:"decision"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || (req.Decision != "approved" && req.Decision != "declined") {
		writeError(w, 400, "choose approve or decline")
		return
	}
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, 500, "could not review request")
		return
	}
	defer tx.Rollback(r.Context())
	q := h.Queries.WithTx(tx)
	if err = q.LockSubscriberWrites(r.Context(), db.LockSubscriberWritesParams{WorkspaceID: ws, UserID: target}); err == nil {
		err = lockWorkspaceMembership(r.Context(), tx, ws)
	}
	if err != nil {
		writeError(w, 404, "workspace not found")
		return
	}
	admin, err := q.GetUser(r.Context(), parseUUID(actor))
	if err != nil {
		writeError(w, 403, "account unavailable")
		return
	}
	authorized := emailAllowedForPermission(admin.Email, h.cfg.PermissionManagerEmails)
	if !authorized {
		m, e := q.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: admin.ID, WorkspaceID: ws})
		authorized = e == nil && (m.Role == "owner" || m.Role == "admin")
	}
	if !authorized {
		writeError(w, 403, "workspace administrator required")
		return
	}
	var status string
	err = tx.QueryRow(r.Context(), `SELECT status FROM workspace_access_request WHERE workspace_id=$1 AND user_id=$2 FOR UPDATE`, ws, target).Scan(&status)
	if err == pgx.ErrNoRows {
		writeError(w, 404, "request not found")
		return
	}
	if err != nil {
		writeError(w, 500, "could not load request")
		return
	}
	if status != "pending" {
		writeError(w, 409, "request has already been resolved; refresh the list")
		return
	}
	user, err := q.GetUser(r.Context(), target)
	if err != nil {
		writeError(w, 404, "requester not found")
		return
	}
	var added *db.Member
	if req.Decision == "approved" {
		_, err = q.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: target, WorkspaceID: ws})
		if err == pgx.ErrNoRows {
			if h.seatCapacityEnabled() {
				writeError(w, 409, "use a workspace invitation to reserve a member seat")
				return
			}
			m, e := q.CreateMember(r.Context(), db.CreateMemberParams{UserID: target, WorkspaceID: ws, Role: "member"})
			err = e
			added = &m
		}
		if err == nil {
			_, err = q.MarkUserOnboarded(r.Context(), target)
		}
		if err != nil {
			writeError(w, 500, "could not add member")
			return
		}
	}
	_, err = tx.Exec(r.Context(), `UPDATE workspace_access_request SET status=$3,decided_by=$4,updated_at=now() WHERE workspace_id=$1 AND user_id=$2`, ws, target, req.Decision, admin.ID)
	if err != nil || tx.Commit(r.Context()) != nil {
		writeError(w, 500, "could not save decision")
		return
	}
	if added != nil {
		wsID, uid := uuidToString(ws), uuidToString(target)
		h.MembershipCache.Invalidate(r.Context(), uid, wsID)
		h.notifyDaemonWorkspacesChanged(uid)
		h.publish(protocol.EventMemberAdded, wsID, "member", actor, map[string]any{"member": h.memberWithUserResponse(*added, user)})
	}
	w.WriteHeader(http.StatusNoContent)
}

package handler

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"net/http"
)

var errLastWorkspaceOwner = errors.New("workspace must have at least one owner")

// Serialize ownership changes across instance management, workspace settings
// and membership revocation so concurrent edits cannot remove the last owner.
func lockWorkspaceMembership(ctx context.Context, tx pgx.Tx, workspaceID pgtype.UUID) error {
	var id pgtype.UUID
	return tx.QueryRow(ctx, `SELECT id FROM workspace WHERE id=$1 FOR UPDATE`, workspaceID).Scan(&id)
}

func protectWorkspaceOwner(ctx context.Context, q *db.Queries, member db.Member, role string) error {
	if member.Role != "owner" || role == "owner" {
		return nil
	}
	members, err := q.ListMembers(ctx, member.WorkspaceID)
	if err != nil {
		return err
	}
	if countOwners(members) <= 1 {
		return errLastWorkspaceOwner
	}
	return nil
}

func (h *Handler) setMemberRole(ctx context.Context, member db.Member, role string) (db.Member, error) {
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return db.Member{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockWorkspaceMembership(ctx, tx, member.WorkspaceID); err != nil {
		return db.Member{}, err
	}
	q := h.Queries.WithTx(tx)
	current, err := q.GetMember(ctx, member.ID)
	if err != nil {
		return db.Member{}, err
	}
	if err = protectWorkspaceOwner(ctx, q, current, role); err != nil {
		return db.Member{}, err
	}
	updated, err := q.UpdateMemberRole(ctx, db.UpdateMemberRoleParams{ID: member.ID, Role: role})
	if err != nil {
		return db.Member{}, err
	}
	return updated, tx.Commit(ctx)
}

func (h *Handler) SetInstanceMembership(w http.ResponseWriter, r *http.Request) {
	if isMachineCredentialActor(r) {
		writeError(w, 403, "human administrator required")
		return
	}
	actor, ok := h.requirePermissionManager(w, r)
	if !ok {
		return
	}
	userID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "userId"), "user id")
	if !ok {
		return
	}
	wsID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "workspaceId"), "workspace id")
	if !ok {
		return
	}
	user, err := h.Queries.GetUser(r.Context(), userID)
	if err != nil {
		writeError(w, 404, "user not found")
		return
	}
	if _, err = h.Queries.GetWorkspace(r.Context(), wsID); err != nil {
		writeError(w, 404, "workspace not found")
		return
	}
	var req struct {
		Role string `json:"role"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || (req.Role != "none" && req.Role != "member" && req.Role != "admin" && req.Role != "owner") {
		writeError(w, 400, "invalid membership role")
		return
	}
	existing, err := h.Queries.GetMemberByUserAndWorkspace(r.Context(), db.GetMemberByUserAndWorkspaceParams{UserID: userID, WorkspaceID: wsID})
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 500, "could not load membership")
		return
	}
	ws := uuidToString(wsID)
	uid := uuidToString(userID)
	if req.Role == "none" {
		if exists {
			result, err := h.revokeAndRemoveMember(r.Context(), wsID, userID, existing.ID, actor)
			if errors.Is(err, errLastWorkspaceOwner) {
				writeError(w, 409, err.Error())
				return
			}
			if err != nil {
				writeError(w, 500, "could not remove membership")
				return
			}
			h.settleMemberCapacityRelease(r.Context(), uuid.UUID(wsID.Bytes), uuid.UUID(existing.ID.Bytes))
			h.publishRevocation(r.Context(), result, ws, "member", uuidToString(actor))
			h.publish(protocol.EventMemberRemoved, ws, "member", uuidToString(actor), map[string]any{"member_id": uuidToString(existing.ID), "workspace_id": ws, "user_id": uid})
		}
	} else {
		var updated db.Member
		event := protocol.EventMemberUpdated
		if exists {
			updated, err = h.setMemberRole(r.Context(), existing, req.Role)
		} else {
			// Paid deployments must keep seat admission on the invitation path.
			if h.seatCapacityEnabled() {
				writeError(w, 409, "use a workspace invitation to reserve a member seat")
				return
			}
			tx, e := h.TxStarter.Begin(r.Context())
			if e != nil {
				writeError(w, 500, "could not add membership")
				return
			}
			defer tx.Rollback(r.Context())
			q := h.Queries.WithTx(tx)
			if e = q.LockSubscriberWrites(r.Context(), db.LockSubscriberWritesParams{WorkspaceID: wsID, UserID: userID}); e == nil {
				e = lockWorkspaceMembership(r.Context(), tx, wsID)
			}
			if e != nil {
				writeError(w, 500, "could not lock membership")
				return
			}
			updated, err = q.CreateMember(r.Context(), db.CreateMemberParams{UserID: userID, WorkspaceID: wsID, Role: req.Role})
			if err == nil {
				_, err = q.MarkUserOnboarded(r.Context(), userID)
			}
			if err == nil {
				err = tx.Commit(r.Context())
			}
			event = protocol.EventMemberAdded
		}
		if errors.Is(err, errLastWorkspaceOwner) {
			writeError(w, 409, err.Error())
			return
		}
		if err != nil {
			writeError(w, 409, "membership changed or could not be saved; reload and retry")
			return
		}
		h.publish(event, ws, "member", uuidToString(actor), map[string]any{"member": h.memberWithUserResponse(updated, user)})
	}
	h.MembershipCache.Invalidate(r.Context(), uid, ws)
	h.notifyDaemonWorkspacesChanged(uid)
	writeJSON(w, 200, map[string]any{"status": "ok"})
}

package workspacerepo

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

type Plan struct {
	Configuration
	WorkspaceID string `json:"workspace_id"`
	Fingerprint string `json:"fingerprint"`
}
type Readiness struct {
	WorkspaceID string    `json:"workspace_id"`
	Fingerprint string    `json:"fingerprint"`
	State       string    `json:"state"`
	Message     string    `json:"message"`
	UpdatedAt   time.Time `json:"updated_at"`
	Usable      bool      `json:"usable"`
}

var ErrConfigurationRequired = errors.New("configure a GitHub repository or local folder in workspace settings before creating issues")

func MachineKey(rt db.AgentRuntime) string {
	return fmt.Sprintf("%s:%s", rt.OwnerID.String(), rt.DaemonID.String)
}
func StatusKey(rt db.AgentRuntime, workspace string) string { return workspace + ":" + MachineKey(rt) }
func Load(ctx context.Context, q *db.Queries, scope, subject string) (Configuration, int64, error) {
	row, err := q.GetRepositoryConfiguration(ctx, db.GetRepositoryConfigurationParams{Scope: scope, Subject: subject})
	if errors.Is(err, pgx.ErrNoRows) {
		return Configuration{}, 0, nil
	}
	if err != nil {
		return Configuration{}, 0, err
	}
	var c Configuration
	if err = json.Unmarshal(row.Config, &c); err == nil {
		err = c.Validate(scope)
	}
	return c, row.Revision, err
}
func RequireConfigured(ctx context.Context, q *db.Queries, workspace string) error {
	c, _, err := Load(ctx, q, "workspace", workspace)
	if err != nil {
		return err
	}
	if c.Repository == "" && c.Folder == "" {
		return ErrConfigurationRequired
	}
	return nil
}
func BuildPlan(ctx context.Context, q *db.Queries, rt db.AgentRuntime, workspace string) (Plan, error) {
	c, rev, err := Load(ctx, q, "workspace", workspace)
	if err != nil {
		return Plan{}, err
	}
	machine, mrev, err := Load(ctx, q, "machine", MachineKey(rt))
	if err != nil {
		return Plan{}, err
	}
	c.Root = machine.Root
	raw, _ := json.Marshal([]any{c, rev, mrev})
	sum := sha256.Sum256(raw)
	return Plan{Configuration: c, WorkspaceID: workspace, Fingerprint: hex.EncodeToString(sum[:])}, nil
}
func Status(ctx context.Context, q *db.Queries, rt db.AgentRuntime, workspace string) (Readiness, error) {
	p, err := BuildPlan(ctx, q, rt, workspace)
	if err != nil {
		return Readiness{}, err
	}
	status := Readiness{WorkspaceID: workspace, Fingerprint: p.Fingerprint, State: "preparing", Message: "Waiting for the computer to check this workspace"}
	if p.Folder == "" {
		status.State = "configuration_required"
		status.Message = ErrConfigurationRequired.Error()
		return status, nil
	}
	var meta struct {
		Version int `json:"workspace_repository_version"`
	}
	_ = json.Unmarshal(rt.Metadata, &meta)
	if meta.Version < 2 {
		status.State = "upgrade_required"
		status.Message = "Update Multica on this computer to prepare workspace repositories"
		return status, nil
	}
	if p.Root == "" {
		status.State = "root_required"
		status.Message = "Set this computer's repositories folder"
		return status, nil
	}
	if rt.Status != "online" || !rt.LastSeenAt.Valid || time.Since(rt.LastSeenAt.Time) > 150*time.Second {
		status.State = "offline"
		status.Message = "Computer is offline"
		return status, nil
	}
	row, err := q.GetRepositoryConfiguration(ctx, db.GetRepositoryConfigurationParams{Scope: "readiness", Subject: StatusKey(rt, workspace)})
	if errors.Is(err, pgx.ErrNoRows) {
		return status, nil
	}
	if err != nil {
		return status, err
	}
	var reported Readiness
	if json.Unmarshal(row.Config, &reported) != nil || reported.Fingerprint != p.Fingerprint || time.Since(reported.UpdatedAt) > 2*time.Minute {
		return status, nil
	}
	reported.Usable = reported.State == "ready"
	return reported, nil
}

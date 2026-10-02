package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/pkg/workflow"
	"github.com/spf13/cobra"
)

func init() {
	issueCmd.AddCommand(&cobra.Command{Use: "doctor", Short: "Inspect tool discovery on this runtime machine", Args: exactArgs(0), RunE: func(cmd *cobra.Command, args []string) error {
		inventory := map[string]any{"runtime_id": os.Getenv("MULTICA_RUNTIME_ID"), "task_id": os.Getenv("MULTICA_TASK_ID")}
		paths := map[string]string{}
		for _, name := range []string{"git", "gh", "fvm", "flutter", "dart", "node", "go"} {
			path, _ := workflow.FindTool(name)
			paths[name] = path
		}
		inventory["tools"] = paths
		inventory["cli_version"] = version
		inventory["observed_at"] = time.Now().UTC()
		host, _ := os.Hostname()
		inventory["machine"] = host
		dir, _ := os.Getwd()
		inventory["work_directory"] = dir
		inventory["shell"] = os.Getenv("SHELL")
		if os.Getenv("COMSPEC") != "" {
			inventory["shell"] = os.Getenv("COMSPEC")
		}
		return cli.PrintJSON(os.Stdout, inventory)
	}})
	contextCmd := &cobra.Command{Use: "context <issue-id>", Short: "Read bounded issue context, decisions, checkpoints and new activity", Args: exactArgs(1), RunE: runIssueContext}
	contextCmd.Flags().String("since", "", "Cursor timestamp from the previous context response")
	contextCmd.Flags().String("output", "json", "Output format: json")
	issueCmd.AddCommand(contextCmd)
	work := &cobra.Command{Use: "work", Short: "Record decisions and blockers"}
	issueCmd.AddCommand(work)
	installs := &cobra.Command{Use: "installation", Short: "Request human approval and resume on the same runtime machine"}
	issueCmd.AddCommand(installs)
	checkpoints := &cobra.Command{Use: "checkpoint", Short: "Claim work branches and save verified remote checkpoints"}
	issueCmd.AddCommand(checkpoints)
	for _, spec := range []struct {
		parent             *cobra.Command
		name, kind, action string
		n                  int
	}{
		{work, "list", "", "list", 1}, {work, "decision", "decision", "create", 1}, {work, "blocker", "blocker", "create", 1}, {work, "resolve", "", "resolve", 2},
		{installs, "request", "installation", "create", 1}, {installs, "approve", "installation", "approve", 2}, {installs, "decline", "installation", "decline", 2}, {installs, "installed", "installation", "installed", 2},
		{checkpoints, "claim", "checkpoint", "claim", 1}, {checkpoints, "save", "checkpoint", "save", 2}, {checkpoints, "resume", "checkpoint", "resume", 2},
	} {
		spec := spec
		use := spec.name + " <issue-id>"
		if spec.n == 2 {
			use += " <record-id>"
		}
		c := &cobra.Command{Use: use, Short: spec.name + " " + spec.kind + " work record", Args: exactArgs(spec.n), RunE: func(cmd *cobra.Command, args []string) error { return runWorkRecord(cmd, args, spec.kind, spec.action) }}
		flags := []string{}
		switch spec.action {
		case "create":
			if spec.kind == "installation" {
				flags = []string{"tool", "version", "source", "scope", "effects", "reason"}
			} else {
				flags = []string{"text", "owner", "next-step"}
			}
		case "claim":
			flags = []string{"repository", "branch"}
		case "save":
			flags = []string{"pr-url", "validation", "next-step"}
		case "installed":
			flags = []string{"verification"}
		}
		for _, flag := range flags {
			c.Flags().String(flag, "", flag+" for this work record")
		}
		if spec.kind == "checkpoint" {
			c.Flags().String("directory", ".", "Task checkout to verify or resume")
		}
		if spec.action == "save" {
			c.Flags().StringArray("dependency", nil, "Additional dependency checkout to verify (repeatable); submodules are checked automatically")
		}
		c.Flags().String("output", "json", "Output format: json")
		spec.parent.AddCommand(c)
	}
	runs := &cobra.Command{Use: "run", Short: "Inspect execution identity and bounded run logs"}
	get := &cobra.Command{Use: "get <issue-id> <run-id>", Short: "Inspect a run and its original trigger", Args: exactArgs(2), RunE: runIssueRunGet}
	logs := &cobra.Command{Use: "logs <run-id>", Short: "Read bounded execution messages", Args: exactArgs(1), RunE: runIssueRunMessages}
	logs.Flags().String("issue", "", "Issue for resolving a short run ID")
	logs.Flags().Int("since", 0, "Read after sequence")
	logs.Flags().Int("tail", 100, "Maximum messages (1–500)")
	logs.Flags().String("type", "", "text, tool_use, tool_result, or error")
	logs.Flags().String("output", "json", "Output format: table or json")
	runs.AddCommand(get, logs)
	issueCmd.AddCommand(runs)
	issueRunMessagesCmd.Flags().Int("tail", 100, "Maximum messages (1–500)")
	issueRunMessagesCmd.Flags().String("type", "", "Filter message type")
	issueRerunCmd.Flags().String("task", "", "Run to replay or continue; defaults to the issue assignee")
	issueRerunCmd.Flags().String("instruction", "", "New continuation instruction; omit to replay the original trigger")
}
func runIssueContext(cmd *cobra.Command, args []string) error {
	c, e := newAPIClient(cmd)
	if e != nil {
		return e
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issue, e := resolveIssueRef(ctx, c, args[0])
	if e != nil {
		return e
	}
	since, _ := cmd.Flags().GetString("since")
	var out any
	if e = c.GetJSON(ctx, "/api/issues/"+issue.ID+"/context?since="+url.QueryEscape(since), &out); e != nil {
		return e
	}
	return cli.PrintJSON(os.Stdout, out)
}

type cliWorkRecord struct {
	ID                 string        `json:"id"`
	Kind               string        `json:"kind"`
	State              string        `json:"state"`
	Revision           int64         `json:"revision"`
	Data               workflow.Data `json:"data"`
	TaskID             string        `json:"task_id,omitempty"`
	RuntimeID          string        `json:"runtime_id,omitempty"`
	AgentID            string        `json:"agent_id,omitempty"`
	MachineOwnerID     string        `json:"machine_owner_id,omitempty"`
	ContinuationTaskID string        `json:"continuation_task_id,omitempty"`
}

func runWorkRecord(cmd *cobra.Command, args []string, kind, action string) error {
	c, e := newAPIClient(cmd)
	if e != nil {
		return e
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issue, e := resolveIssueRef(ctx, c, args[0])
	if e != nil {
		return e
	}
	path := "/api/issues/" + issue.ID + "/work-records"
	var list struct {
		Records   []cliWorkRecord `json:"records"`
		Truncated bool            `json:"truncated"`
	}
	if e = c.GetJSON(ctx, path, &list); e != nil {
		return e
	}
	if action == "list" {
		return cli.PrintJSON(os.Stdout, list)
	}
	data := workflow.Data{}
	raw := map[string]string{}
	for _, name := range []string{"tool", "version", "source", "scope", "effects", "reason", "verification", "repository", "branch", "pr-url", "validation", "next-step", "owner", "text"} {
		s, _ := cmd.Flags().GetString(name)
		raw[strings.ReplaceAll(name, "-", "_")] = s
	}
	b, _ := json.Marshal(raw)
	_ = json.Unmarshal(b, &data)
	var row cliWorkRecord
	if len(args) == 2 {
		for _, r := range list.Records {
			if r.ID == args[1] {
				row = r
				break
			}
		}
		if row.ID == "" {
			return fmt.Errorf("record not in bounded issue response; reload issue context")
		}
	}
	if action == "claim" {
		if e = data.Validate(kind); e != nil {
			return e
		}
		for _, r := range list.Records {
			if r.Kind == "checkpoint" && r.Data.Repository == data.Repository && r.Data.Branch == data.Branch {
				row = r
				break
			}
		}
		if row.ID == "" {
			action = "create"
		}
	}
	if action == "resume" {
		if row.Kind != "checkpoint" || row.Data.Commit == "" {
			return fmt.Errorf("a saved remote checkpoint is required")
		}
		var claimed cliWorkRecord
		if e = c.PutJSON(ctx, path+"/"+row.ID, map[string]any{"action": "claim", "revision": row.Revision}, &claimed); e != nil {
			return e
		}
		dir, _ := cmd.Flags().GetString("directory")
		if e = resumeWorkCheckpoint(dir, row.Data); e != nil {
			return e
		}
		return cli.PrintJSON(os.Stdout, claimed)
	}
	if action == "save" {
		dir, _ := cmd.Flags().GetString("directory")
		data.Repository = row.Data.Repository
		data.Branch = row.Data.Branch
		data.Commit, e = verifiedRemoteCheckpoint(dir, data)
		if e != nil {
			return e
		}
		data.Dependencies, e = verifiedSubmoduleCheckpoints(dir, 0)
		if e != nil {
			return e
		}
		deps, _ := cmd.Flags().GetStringArray("dependency")
		for _, depDir := range deps {
			repository, err := checkpointRepository(depDir)
			if err != nil {
				return err
			}
			branch, err := checkpointGit(depDir, "branch", "--show-current")
			if err != nil {
				return err
			}
			commit, err := verifiedRemoteCheckpoint(depDir, workflow.Data{Repository: repository, Branch: branch})
			if err != nil {
				return err
			}
			data.Dependencies = append(data.Dependencies, workflow.Dependency{Repository: repository, Commit: commit})
		}
		if e = data.Validate("checkpoint"); e != nil {
			return e
		}
	}
	var out any
	if action == "create" {
		e = c.PostJSON(ctx, path, map[string]any{"kind": kind, "data": data}, &out)
	} else {
		e = c.PutJSON(ctx, path+"/"+row.ID, map[string]any{"action": action, "revision": row.Revision, "data": data}, &out)
	}
	if e != nil {
		return e
	}
	return cli.PrintJSON(os.Stdout, out)
}
func checkpointGit(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.longpaths=true", "-c", "credential.interactive=false", "-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	out, e := cmd.Output()
	if e != nil {
		return "", fmt.Errorf("git %s failed; inspect this checkout's access and branch state", args[0])
	}
	return strings.TrimSpace(string(out)), nil
}
func checkCheckpointRepository(dir string, d workflow.Data) error {
	if e := d.Validate("checkpoint"); e != nil {
		return e
	}
	remote, e := checkpointGit(dir, "config", "--get", "remote.origin.url")
	if e != nil {
		return e
	}
	canonical := strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")
	if canonical != "https://github.com/"+d.Repository && canonical != "git@github.com:"+d.Repository {
		return fmt.Errorf("origin must match checkpoint repository %s", d.Repository)
	}
	return nil
}

func checkpointRepository(dir string) (string, error) {
	remote, e := checkpointGit(dir, "config", "--get", "remote.origin.url")
	if e != nil {
		return "", e
	}
	canonical := strings.TrimSuffix(strings.TrimSuffix(remote, "/"), ".git")
	for _, prefix := range []string{"https://github.com/", "git@github.com:"} {
		if strings.HasPrefix(canonical, prefix) {
			return strings.TrimPrefix(canonical, prefix), nil
		}
	}
	return "", fmt.Errorf("dependency origin must identify a GitHub repository")
}

// Verify gitlinks before advertising the parent as portable. Fetch updates only
// remote tracking refs; it never resets or checks out a dependency worktree.
func verifiedSubmoduleCheckpoints(dir string, depth int) ([]workflow.Dependency, error) {
	if depth > 8 {
		return nil, fmt.Errorf("submodule nesting exceeds verification bound")
	}
	tree, e := checkpointGit(dir, "ls-tree", "-rz", "HEAD")
	if e != nil {
		return nil, e
	}
	out := []workflow.Dependency{}
	for _, entry := range strings.Split(tree, "\x00") {
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 || fields[0] != "160000" {
			continue
		}
		clean := filepath.Clean(filepath.FromSlash(path))
		if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("invalid submodule path")
		}
		sub := filepath.Join(dir, clean)
		repository, err := checkpointRepository(sub)
		if err != nil {
			return nil, err
		}
		head, err := checkpointGit(sub, "rev-parse", "HEAD")
		if err != nil || head != fields[2] {
			return nil, fmt.Errorf("submodule %s must be at the parent gitlink before checkpointing", path)
		}
		dirty, err := checkpointGit(sub, "status", "--porcelain")
		if err != nil || dirty != "" {
			return nil, fmt.Errorf("preserve and push pending submodule work in %s first", path)
		}
		if _, err = checkpointGit(sub, "fetch", "--prune", "origin"); err != nil {
			return nil, err
		}
		refs, err := checkpointGit(sub, "for-each-ref", "--format=%(refname)", "--contains="+head, "refs/remotes/origin/")
		if err != nil || strings.TrimSpace(refs) == "" {
			return nil, fmt.Errorf("push dependency %s before saving the parent checkpoint", repository)
		}
		out = append(out, workflow.Dependency{Repository: repository, Commit: head})
		nested, err := verifiedSubmoduleCheckpoints(sub, depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, nested...)
		if len(out) > 20 {
			return nil, fmt.Errorf("at most 20 dependency checkpoints are supported")
		}
	}
	return out, nil
}
func verifiedRemoteCheckpoint(dir string, d workflow.Data) (string, error) {
	if e := checkCheckpointRepository(dir, d); e != nil {
		return "", e
	}
	branch, e := checkpointGit(dir, "branch", "--show-current")
	if e != nil || branch != d.Branch {
		return "", fmt.Errorf("check out the claimed work branch before saving")
	}
	status, e := checkpointGit(dir, "diff", "--name-only", "HEAD")
	if e != nil || status != "" {
		return "", fmt.Errorf("commit intended tracked changes before saving a checkpoint")
	}
	head, e := checkpointGit(dir, "rev-parse", "HEAD")
	if e != nil {
		return "", e
	}
	refs, e := checkpointGit(dir, "ls-remote", "--exit-code", "origin", "refs/heads/"+d.Branch)
	if e != nil || !strings.HasPrefix(refs, head+"\t") {
		return "", fmt.Errorf("push this work branch first; remote HEAD does not match local HEAD")
	}
	return head, nil
}
func resumeWorkCheckpoint(dir string, d workflow.Data) error {
	if e := checkCheckpointRepository(dir, d); e != nil {
		return e
	}
	status, e := checkpointGit(dir, "status", "--porcelain")
	if e != nil || status != "" {
		return fmt.Errorf("checkpoint resume requires a clean task checkout; preserve local changes first")
	}
	if _, e = checkpointGit(dir, "fetch", "origin", "refs/heads/"+d.Branch); e != nil {
		return e
	}
	if _, e = checkpointGit(dir, "merge-base", "--is-ancestor", d.Commit, "FETCH_HEAD"); e != nil {
		return fmt.Errorf("remote branch no longer contains the recorded checkpoint; reconcile before continuing")
	}
	if _, e = checkpointGit(dir, "show-ref", "--verify", "--quiet", "refs/heads/"+d.Branch); e == nil {
		if _, e = checkpointGit(dir, "switch", d.Branch); e != nil {
			return e
		}
		_, e = checkpointGit(dir, "merge", "--ff-only", "FETCH_HEAD")
		return e
	}
	_, e = checkpointGit(dir, "switch", "-c", d.Branch, "FETCH_HEAD")
	return e
}
func runIssueRunGet(cmd *cobra.Command, args []string) error {
	c, e := newAPIClient(cmd)
	if e != nil {
		return e
	}
	ctx, cancel := cli.APIContext(context.Background())
	defer cancel()
	issue, e := resolveIssueRef(ctx, c, args[0])
	if e != nil {
		return e
	}
	run, e := resolveTaskRunID(ctx, c, issue.ID, args[1])
	if e != nil {
		return e
	}
	var rows []map[string]any
	if e = c.GetJSON(ctx, "/api/issues/"+issue.ID+"/task-runs", &rows); e != nil {
		return e
	}
	for _, row := range rows {
		if strVal(row, "id") == run.ID {
			return cli.PrintJSON(os.Stdout, row)
		}
	}
	return fmt.Errorf("run not found")
}

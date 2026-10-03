package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/multica-ai/multica/server/pkg/runtimecap"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// ReadCodexCapabilities uses supported read APIs only: no thread/turn, plugin
// installation, authentication refresh, or experimental plugin/list request.
func ReadCodexCapabilities(ctx context.Context, runtimeCmd Command) (runtimecap.Report, error) {
	r := runtimecap.New("codex")
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := runtimeCmd.exec(ctx, "app-server")
	hideAgentWindow(cmd)
	cmd.Dir = os.TempDir()
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	in, e := cmd.StdinPipe()
	if e != nil {
		return r, e
	}
	out, e := cmd.StdoutPipe()
	if e != nil {
		in.Close()
		return r, e
	}
	if e = startOwnedProcessTree(cmd, runtimeCmd.logger); e != nil {
		in.Close()
		out.Close()
		return r, e
	}
	defer func() {
		in.Close()
		cancel()
		signalProcessGroup(cmd, syscall.SIGKILL)
		out.Close()
		_ = cmd.Wait()
		releaseProcessGroup(cmd)
	}()
	scan := bufio.NewScanner(out)
	scan.Buffer(make([]byte, 4096), 1024*1024)
	request := func(id int, method string, params any) (json.RawMessage, error) {
		if e := json.NewEncoder(in).Encode(map[string]any{"id": id, "method": method, "params": params}); e != nil {
			return nil, e
		}
		for scan.Scan() {
			var msg struct {
				ID     int             `json:"id"`
				Result json.RawMessage `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			if json.Unmarshal(scan.Bytes(), &msg) != nil || msg.ID != id {
				continue
			}
			if len(msg.Error) > 0 && string(msg.Error) != "null" {
				return nil, fmt.Errorf("capability RPC failed")
			}
			return msg.Result, nil
		}
		return nil, fmt.Errorf("capability RPC ended")
	}
	if _, e = request(1, "initialize", map[string]any{"clientInfo": map[string]string{"name": "multica-capabilities", "version": "1"}}); e != nil {
		return r, e
	}
	if e = json.NewEncoder(in).Encode(map[string]any{"method": "initialized"}); e != nil {
		return r, e
	}

	skills, skillErr := request(2, "skills/list", map[string]any{"cwds": []string{os.TempDir()}, "forceReload": true})
	if skillErr == nil {
		var v struct {
			Data []struct {
				Skills []struct {
					Name        string
					Description string
					Scope       string
					Path        string
					Enabled     *bool
				}
				Errors []json.RawMessage
			}
		}
		if json.Unmarshal(skills, &v) == nil && len(v.Data) > 0 {
			r.Status = "reported"
			plugins := map[string]bool{}
			for _, group := range v.Data {
				if len(group.Errors) > 0 {
					r.Truncated = true
				}
				for _, s := range group.Skills {
					if s.Scope == "repo" || s.Scope == "project" {
						continue
					}
					// Only the provider's home-level inventory is portable. A system temp
					// directory is not a project; repository-scoped results are omitted.
					plugin := ""
					parts := strings.Split(filepath.ToSlash(s.Path), "/plugins/cache/")
					if len(parts) == 2 {
						segments := strings.Split(parts[1], "/")
						if len(segments) >= 3 {
							plugin = segments[1] + "@" + segments[0]
						}
					}
					if plugin != "" && !plugins[plugin] {
						plugins[plugin] = true
						r.Add(runtimecap.Entry{Kind: "plugin", Name: plugin, Enabled: s.Enabled})
					}
					r.Add(runtimecap.Entry{Kind: "skill", Name: s.Name, Description: s.Description, Plugin: plugin, Enabled: s.Enabled})
				}
			}
		}
	}
	apps, appErr := request(3, "app/installed", map[string]any{"forceRefresh": false})
	if appErr == nil {
		var v struct {
			Apps []struct {
				ID          string
				RuntimeName string
				Enabled     *bool
				Callable    *bool
			}
		}
		if json.Unmarshal(apps, &v) == nil && v.Apps != nil {
			r.Status = "reported"
			for _, a := range v.Apps {
				name := a.RuntimeName
				if name == "" {
					name = a.ID
				}
				r.Add(runtimecap.Entry{Kind: "app", Name: name, Enabled: a.Enabled, Callable: a.Callable})
			}
		}
	}
	r.ObservedAt = time.Now().UTC()
	return r, nil
}

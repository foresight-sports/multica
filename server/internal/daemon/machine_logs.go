package daemon

import (
	"context"
	"io"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/multica-ai/multica/server/pkg/redact"
)

// Only the CLI's configured log sinks are readable; the server cannot ask for
// arbitrary paths. Each short-lived read also tolerates log rotation.
func readMachineLogTail(path string, limit int) (string, bool, error) {
	if path == "" {
		return "", false, nil
	}
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", false, err
	}
	offset := info.Size() - int64(limit)
	truncated := offset > 0
	if offset < 0 {
		offset = 0
	}
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return "", false, err
	}
	raw, err := io.ReadAll(io.LimitReader(f, int64(limit)))
	if err != nil {
		return "", false, err
	}
	if truncated {
		if i := strings.IndexByte(string(raw), '\n'); i >= 0 {
			raw = raw[i+1:]
		} else {
			raw = nil
		}
	}
	text := redact.Text(strings.ToValidUTF8(string(raw), ""))
	if len(text) > limit {
		text = text[:limit]
		for !utf8.ValidString(text) {
			text = text[:len(text)-1]
		}
	}
	return text, truncated, nil
}

func (d *Daemon) machineLogsLoop(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		ids := d.allRuntimeIDs()
		if len(ids) > 0 {
			log, truncated, err := readMachineLogTail(d.cfg.LogPath, 64*1024)
			crash, crashTruncated, crashErr := readMachineLogTail(d.cfg.CrashLogPath, 16*1024)
			message := ""
			if d.cfg.LogPath == "" {
				message = "This daemon is logging to the terminal; restart it in background mode to collect logs."
			}
			if err != nil || crashErr != nil {
				message = "One or more daemon log files could not be read."
			}
			reportCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			// One snapshot per machine, regardless of its number of runtimes/workspaces.
			_ = d.client.postJSON(reportCtx, "/api/daemon/runtimes/"+ids[0]+"/logs", map[string]any{"log": log, "crash": crash, "message": message, "truncated": truncated || crashTruncated}, nil)
			cancel()
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

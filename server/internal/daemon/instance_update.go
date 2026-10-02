package daemon

import (
	"context"
	"fmt"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/pkg/daemonrelease"
	"io"
	"net/http"
	"runtime"
	"time"
)

func (d *Daemon) runInstanceUpdate(target string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	data, checksum, err := d.downloadInstanceUpdate(ctx, target)
	if err != nil {
		return "", err
	}
	d.instanceUpdateProgress("Download complete. Verifying SHA-256 and installing executable.")
	output, err := cli.InstallInstanceBinary(data, checksum)
	if err == nil {
		d.instanceUpdateProgress("Checksum verified; executable replaced successfully.")
	}
	return output, err
}

func (d *Daemon) instanceUpdateProgress(message string) {
	if report := d.updateProgress.Load(); report != nil {
		(*report)(message)
	}
}

// Download only from the configured instance; installing happens after validation.
func (d *Daemon) downloadInstanceUpdate(ctx context.Context, target string) ([]byte, string, error) {
	var m daemonrelease.Manifest
	d.instanceUpdateProgress("Loading the instance release manifest.")
	if e := d.client.getJSON(ctx, "/api/daemon/release", &m); e != nil {
		return nil, "", fmt.Errorf("instance release unavailable")
	}
	if m.Version != target || !daemonrelease.Newer(target, d.cfg.CLIVersion) {
		return nil, "", fmt.Errorf("release changed or target is not newer")
	}
	a, ok := m.Assets[runtime.GOOS+"-"+runtime.GOARCH]
	if !ok || a.Size <= 0 || a.Size > daemonrelease.MaxBinarySize {
		return nil, "", fmt.Errorf("no valid artifact for this platform")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", d.client.baseURL+"/api/daemon/release/"+target+"/"+runtime.GOOS+"/"+runtime.GOARCH, nil)
	if e != nil {
		return nil, "", e
	}
	req.Header.Set("Authorization", "Bearer "+d.client.token)
	d.client.setIdentityHeaders(req)
	downloadClient := *d.client.client
	downloadClient.Timeout = 3 * time.Minute
	downloadClient.CheckRedirect = func(*http.Request, []*http.Request) error { return fmt.Errorf("release redirects are not allowed") }
	resp, e := downloadClient.Do(req)
	if e != nil {
		return nil, "", fmt.Errorf("instance artifact download failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("instance artifact returned HTTP %d", resp.StatusCode)
	}
	d.instanceUpdateProgress(fmt.Sprintf("Downloading %s (%d bytes).", target, a.Size))
	reader := &instanceUpdateReader{reader: io.LimitReader(resp.Body, a.Size+1), total: a.Size, report: d.instanceUpdateProgress}
	data, e := io.ReadAll(reader)
	if e != nil || int64(len(data)) != a.Size {
		return nil, "", fmt.Errorf("instance artifact size mismatch")
	}
	return data, a.SHA256, nil
}

type instanceUpdateReader struct {
	reader          io.Reader
	total, received int64
	last            time.Time
	report          func(string)
}

func (r *instanceUpdateReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.received += int64(n)
	if time.Since(r.last) >= 2*time.Second || err == io.EOF {
		r.report(fmt.Sprintf("Downloaded %d of %d bytes.", r.received, r.total))
		r.last = time.Now()
	}
	return n, err
}

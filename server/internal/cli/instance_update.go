package cli

import (
	"fmt"
	"github.com/multica-ai/multica/server/internal/selfexec"
	"path/filepath"
)

func InstallInstanceBinary(data []byte, sha string) (string, error) {
	if e := verifyAssetSHA256(data, sha, "instance daemon"); e != nil {
		return "", e
	}
	exe, e := selfexec.Resolve()
	if e != nil {
		return "", e
	}
	exe, e = filepath.EvalSymlinks(exe)
	if e != nil {
		return "", e
	}
	if len(data) == 0 {
		return "", fmt.Errorf("empty binary")
	}
	return installBinary(data, exe, "instance daemon")
}

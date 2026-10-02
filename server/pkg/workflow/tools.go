package workflow

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// ToolPath adds stable, current-user installation locations. It never edits
// the user's environment or searches another machine/account's directories.
func ToolPath() string {
	home, _ := os.UserHomeDir()
	dirs := filepath.SplitList(os.Getenv("PATH"))
	if home != "" {
		dirs = append(dirs, filepath.Join(home, ".multica", "tools", "bin"), filepath.Join(home, ".local", "bin"), filepath.Join(home, ".pub-cache", "bin"))
	}
	if runtime.GOOS == "windows" && os.Getenv("LOCALAPPDATA") != "" {
		dirs = append(dirs, filepath.Join(os.Getenv("LOCALAPPDATA"), "Pub", "Cache", "bin"))
	}
	seen := map[string]bool{}
	out := []string{}
	for _, dir := range dirs {
		if dir == "" || !filepath.IsAbs(dir) || seen[dir] {
			continue
		}
		seen[dir] = true
		out = append(out, dir)
	}
	return strings.Join(out, string(os.PathListSeparator))
}

func FindTool(name string) (string, error) {
	if strings.ContainsAny(name, "/\\:") {
		return "", exec.ErrNotFound
	}
	for _, dir := range filepath.SplitList(ToolPath()) {
		if path, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return path, nil
		}
	}
	return "", exec.ErrNotFound
}

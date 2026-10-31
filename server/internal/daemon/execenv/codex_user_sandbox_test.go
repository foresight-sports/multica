package execenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestWindowsTaskHonorsExplicitHostAccessMode(t *testing.T) {
	for _, tc := range []struct{ name, config, want string }{
		{"full access with elevated implementation", "sandbox_mode = 'danger-full-access'\n[windows]\nsandbox = 'elevated'\n", "danger-full-access"},
		{"read only with elevated implementation", "sandbox_mode = 'read-only'\n[windows]\nsandbox = 'elevated'\n", "read-only"},
		{"workspace with elevated implementation", "sandbox_mode = 'workspace-write'\n[windows]\nsandbox = 'elevated'\n", "workspace-write"},
		{"implementation alone remains isolated", "[windows]\nsandbox = 'elevated'\n", "workspace-write"},
		{"invalid implementation fails closed", "sandbox_mode = 'danger-full-access'\n[windows]\nsandbox = 'invalid'\n", "workspace-write"},
		{"managed fallback is not user intent", multicaManagedBeginMarker + "\nsandbox_mode = 'danger-full-access'\n" + multicaManagedEndMarker + "\n[windows]\nsandbox = 'elevated'\n", "workspace-write"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared := t.TempDir()
			t.Setenv("CODEX_HOME", shared)
			if err := os.WriteFile(filepath.Join(shared, "config.toml"), []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			taskHome := filepath.Join(t.TempDir(), "codex-home")
			for attempt := 0; attempt < 2; attempt++ {
				if err := prepareCodexHomeWithOpts(taskHome, CodexHomeOptions{GOOS: "windows", CodexVersion: "0.153.4"}, testLogger()); err != nil {
					t.Fatal(err)
				}
				data, err := os.ReadFile(filepath.Join(taskHome, "config.toml"))
				if err != nil {
					t.Fatal(err)
				}
				var result struct {
					Mode    string `toml:"sandbox_mode"`
					Windows struct {
						Sandbox string `toml:"sandbox"`
					} `toml:"windows"`
				}
				if err := toml.Unmarshal(data, &result); err != nil {
					t.Fatal(err)
				}
				if result.Mode != tc.want {
					t.Fatalf("mode=%s want %s", result.Mode, tc.want)
				}
			}
			original, _ := os.ReadFile(filepath.Join(shared, "config.toml"))
			if string(original) != tc.config {
				t.Fatal("modified shared user config")
			}
		})
	}
}

func TestWindowsUserSandboxPolicyUnavailableConfigKeepsIsolation(t *testing.T) {
	fallback := codexSandboxPolicyForWindows(windowsSandboxNative)
	for _, body := range []string{"sandbox_mode = [", "sandbox_mode = 'unknown'"} {
		path := filepath.Join(t.TempDir(), "config.toml")
		os.WriteFile(path, []byte(body), 0600)
		if got := codexWindowsUserSandboxPolicy(path, fallback); got.Mode != "workspace-write" {
			t.Fatalf("unsafe mode %s", got.Mode)
		}
	}
	if got := codexWindowsUserSandboxPolicy(filepath.Join(t.TempDir(), "missing"), fallback); got.Mode != "workspace-write" {
		t.Fatalf("unsafe mode %s", got.Mode)
	}
}

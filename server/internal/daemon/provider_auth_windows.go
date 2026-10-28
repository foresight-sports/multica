//go:build windows

package daemon

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"

	"golang.org/x/sys/windows/registry"
)

func powershellLiteral(value string) string { return "'" + strings.ReplaceAll(value, "'", "''") + "'" }
func providerAuthCapabilityVersion() int    { return 1 }
func encodePowerShell(value string) string {
	units := utf16.Encode([]rune(value))
	raw := make([]byte, len(units)*2)
	for i, v := range units {
		binary.LittleEndian.PutUint16(raw[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func providerAuthLauncher(encoded string) string {
	// Start-Process -Wait waits for descendants, including browsers launched
	// during OAuth. Only the setup shell's exit determines setup completion.
	return "$p = Start-Process -FilePath powershell.exe -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-EncodedCommand'," + powershellLiteral(encoded) + ") -WindowStyle Normal -PassThru; $p.WaitForExit(); exit $p.ExitCode"
}

func runProviderAuth(ctx context.Context, provider, executable string) error {
	if provider != "claude" && provider != "codex" {
		return fmt.Errorf("unsupported provider")
	}
	dir, err := os.MkdirTemp("", "multica-provider-auth-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	script := filepath.Join(dir, "setup.ps1")
	if err = os.WriteFile(script, providerAuthScript, 0600); err != nil {
		return err
	}
	// The owner explicitly requested an interactive setup window from the UI.
	// The outer process is hidden; only the sign-in window is visible. No stdout
	// or stderr is captured by the daemon or uploaded to the server.
	inner := "& " + powershellLiteral(script) + " -Provider " + powershellLiteral(provider) + " -Executable " + powershellLiteral(executable) + "; exit $LASTEXITCODE"
	encoded := encodePowerShell(inner)
	outer := providerAuthLauncher(encoded)
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-EncodedCommand", encodePowerShell(outer))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	cmd.Cancel = func() error {
		kill := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		return kill.Run()
	}
	return cmd.Run()
}

// Read the owner-persisted token for each new run so setup takes effect without
// restarting the daemon or putting the token in a server-managed profile.
func applyProviderAuthEnvironment(provider string, env map[string]string) {
	if provider != "claude" {
		return
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	token, _, err := key.GetStringValue("CLAUDE_CODE_OAUTH_TOKEN")
	if err == nil && strings.HasPrefix(token, "sk-ant-oat") {
		env["CLAUDE_CODE_OAUTH_TOKEN"] = token
	}
}

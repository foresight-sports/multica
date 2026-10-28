//go:build windows

package daemon

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unicode/utf16"
)

func TestProviderAuthCompletionDoesNotWaitForBrowserDescendants(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "descendant.pid")
	// Simulate a browser outliving the setup terminal. All test windows stay
	// hidden and no installed agent CLI or authentication state is accessed.
	inner := "$child = Start-Process powershell.exe -ArgumentList @('-NoProfile','-Command','Start-Sleep -Seconds 30') -WindowStyle Hidden -PassThru; Set-Content -LiteralPath " + powershellLiteral(pidFile) + " -Value $child.Id; exit 7"
	outer := strings.Replace(providerAuthLauncher(encodePowerShell(inner)), "-WindowStyle Normal", "-WindowStyle Hidden", 1)
	t.Cleanup(func() {
		raw, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
		if err != nil {
			return
		}
		kill := exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid))
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		_ = kill.Run()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-EncodedCommand", encodePowerShell(outer))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatal("setup waited for its browser descendant")
	}
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 7 {
		t.Fatalf("setup exit code was lost: %v", err)
	}
}

func TestProviderAuthUpdateFailureStillStartsLogin(t *testing.T) {
	for _, failure := range []string{"$global:LASTEXITCODE = 1", "throw 'registry unavailable'"} {
		t.Run(failure, func(t *testing.T) {
			dir := t.TempDir()
			script := filepath.Join(dir, "setup.ps1")
			fake := filepath.Join(dir, "fake-claude.ps1")
			calls := filepath.Join(dir, "calls.txt")
			if err := os.WriteFile(script, providerAuthScript, 0600); err != nil {
				t.Fatal(err)
			}
			// Stop at a simulated authorization failure, before any credential input
			// or persistence. Never invoke a real provider CLI in this test.
			body := "param([string]$Action)\nAdd-Content -LiteralPath " + powershellLiteral(calls) + " -Value $Action\nif ($Action -eq 'update') { " + failure + "; return }; $global:LASTEXITCODE = 1"
			if err := os.WriteFile(fake, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			command := "function Start-Sleep {}\n& " + powershellLiteral(script) + " -Provider claude -Executable " + powershellLiteral(fake)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encodePowerShell(command))
			for _, value := range os.Environ() {
				key, _, _ := strings.Cut(value, "=")
				if !strings.HasPrefix(strings.ToUpper(key), "ANTHROPIC_") && !strings.HasPrefix(strings.ToUpper(key), "CLAUDE_") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("simulated authorization failure must not report success")
			}
			raw, err := os.ReadFile(calls)
			if err != nil {
				t.Fatalf("missing fake CLI calls: %v; %s", err, output)
			}
			if strings.Join(strings.Fields(string(raw)), ",") != "update,setup-token" {
				t.Fatalf("login was blocked: %q; %s", raw, output)
			}
		})
	}
}

func TestProviderAuthPowerShellEncoding(t *testing.T) {
	// Paths are data, including spaces, quotes, non-ASCII and PowerShell syntax.
	path := `C:\Users\O'Brien $env:SECRET\日本\claude.cmd`
	literal := powershellLiteral(path)
	if literal != `'C:\Users\O''Brien $env:SECRET\日本\claude.cmd'` {
		t.Fatal("unsafe literal")
	}
	encoded := encodePowerShell(literal)
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	units := make([]uint16, len(raw)/2)
	for i := range units {
		units[i] = binary.LittleEndian.Uint16(raw[i*2:])
	}
	if string(utf16.Decode(units)) != literal {
		t.Fatal("encoding changed command")
	}
	if strings.Contains(encoded, "SECRET") {
		t.Fatal("expected encoded argument")
	}
}

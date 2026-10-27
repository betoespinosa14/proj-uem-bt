// Package exec runs remote scripts and device actions on behalf of the server.
package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/uem/uem/internal/protocol"
)

// cappedBuffer keeps the first MaxOutputBytes of output and discards the rest.
type cappedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	truncated bool
}

func (c *cappedBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	room := protocol.MaxOutputBytes - c.buf.Len()
	if room <= 0 {
		c.truncated = true
		return len(p), nil
	}
	if len(p) > room {
		c.buf.Write(p[:room])
		c.truncated = true
		return len(p), nil
	}
	return c.buf.Write(p)
}

func (c *cappedBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.buf.String()
	if c.truncated {
		s += "\n[output truncated]"
	}
	return s
}

// interpreter returns the command line and temp-file extension for a shell.
func interpreter(shell string) (bin string, args []string, ext string, err error) {
	switch shell {
	case "bash", "sh", "zsh":
		if runtime.GOOS == "windows" {
			return "", nil, "", fmt.Errorf("%s is not available on Windows", shell)
		}
		return shell, nil, ".sh", nil
	case "powershell":
		if runtime.GOOS == "windows" {
			return "powershell.exe", []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File"}, ".ps1", nil
		}
		if _, err := osexec.LookPath("pwsh"); err == nil {
			return "pwsh", []string{"-NoProfile", "-NonInteractive", "-File"}, ".ps1", nil
		}
		return "", nil, "", errors.New("PowerShell (pwsh) is not installed on this device")
	case "cmd":
		if runtime.GOOS != "windows" {
			return "", nil, "", errors.New("cmd is only available on Windows")
		}
		return "cmd.exe", []string{"/Q", "/C"}, ".bat", nil
	}
	return "", nil, "", fmt.Errorf("unsupported shell %q", shell)
}

// RunScript writes the script to a private temp file and executes it with
// the requested interpreter, enforcing the timeout.
func RunScript(ctx context.Context, p protocol.RunScriptPayload) protocol.CommandResult {
	res := protocol.CommandResult{Status: protocol.StatusFailed}
	bin, args, ext, err := interpreter(p.Shell)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if _, err := osexec.LookPath(bin); err != nil {
		res.Error = fmt.Sprintf("%s not found on this device", bin)
		return res
	}
	dir, err := os.MkdirTemp("", "uem-script-")
	if err != nil {
		res.Error = err.Error()
		return res
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "script"+ext)
	body := p.Script
	if ext == ".ps1" {
		// Make PowerShell surface script errors as a non-zero exit code.
		body = "$ErrorActionPreference = 'Stop'\n" + body
	}
	if ext == ".bat" || ext == ".ps1" {
		body = strings.ReplaceAll(body, "\r\n", "\n")
		body = strings.ReplaceAll(body, "\n", "\r\n")
	}
	if err := os.WriteFile(path, []byte(body), 0o700); err != nil {
		res.Error = err.Error()
		return res
	}

	timeout := time.Duration(p.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := osexec.CommandContext(ctx, bin, append(args, path)...)
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second // don't hang on grandchildren holding the pipes
	configureProcess(cmd)
	var stdout, stderr cappedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	res.Stdout, res.Stderr = stdout.String(), stderr.String()
	if cmd.ProcessState != nil {
		code := cmd.ProcessState.ExitCode()
		res.ExitCode = &code
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		res.Status = protocol.StatusTimeout
		res.Error = fmt.Sprintf("script exceeded its %s timeout", timeout)
	case err == nil:
		res.Status = protocol.StatusSucceeded
	default:
		var exitErr *osexec.ExitError
		if !errors.As(err, &exitErr) {
			res.Error = err.Error()
		}
	}
	return res
}

// Power schedules a reboot or shutdown after delay.
func Power(reboot bool, delay time.Duration) error {
	secs := int(delay.Seconds())
	switch runtime.GOOS {
	case "windows":
		flag := "/s"
		if reboot {
			flag = "/r"
		}
		return osexec.Command("shutdown.exe", flag, "/t", fmt.Sprint(secs), "/c", "Scheduled by your IT administrator").Run()
	default:
		flag := "-h"
		if reboot {
			flag = "-r"
		}
		when := "now"
		if secs > 0 {
			when = fmt.Sprintf("+%d", (secs+59)/60) // shutdown(8) takes minutes
		}
		return osexec.Command("shutdown", flag, when).Run()
	}
}

// Lock locks the interactive session(s).
func Lock(ctx context.Context) error {
	switch runtime.GOOS {
	case "windows":
		// The agent runs as SYSTEM in session 0; disconnecting the console
		// session returns it to the lock screen.
		return runCmd(ctx, "tsdiscon.exe", "console")
	case "darwin":
		return runCmd(ctx, "pmset", "displaysleepnow")
	default:
		return runCmd(ctx, "loginctl", "lock-sessions")
	}
}

// DisplayMessage shows a message to logged-in users.
func DisplayMessage(ctx context.Context, title, message string) error {
	switch runtime.GOOS {
	case "windows":
		return runCmd(ctx, "msg.exe", "*", "/TIME:600", title+": "+message)
	case "darwin":
		uid, err := runOut(ctx, "stat", "-f", "%u", "/dev/console")
		if err != nil || uid == "0" {
			return errors.New("no user is logged in at the console")
		}
		script := fmt.Sprintf(`display dialog %s with title %s buttons {"OK"} default button "OK"`, appleQuote(message), appleQuote(title))
		// Fire and forget: the dialog blocks until the user dismisses it.
		return osexec.Command("launchctl", "asuser", uid, "osascript", "-e", script).Start()
	default:
		if _, err := osexec.LookPath("wall"); err != nil {
			return errors.New("wall is not available")
		}
		cmd := osexec.CommandContext(ctx, "wall")
		cmd.Stdin = strings.NewReader(title + "\n\n" + message + "\n")
		return cmd.Run()
	}
}

func appleQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func runCmd(ctx context.Context, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	out, err := osexec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func runOut(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := osexec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

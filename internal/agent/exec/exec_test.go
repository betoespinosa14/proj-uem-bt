package exec

import (
	"context"
	"strings"
	"testing"

	"github.com/uem/uem/internal/protocol"
)

func TestRunScriptCapturesOutputAndExitCode(t *testing.T) {
	res := RunScript(context.Background(), protocol.RunScriptPayload{Shell: "sh", Script: "echo hello; echo oops >&2; exit 3", TimeoutSeconds: 10})
	if res.Status != protocol.StatusFailed || res.ExitCode == nil || *res.ExitCode != 3 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if strings.TrimSpace(res.Stdout) != "hello" || strings.TrimSpace(res.Stderr) != "oops" {
		t.Fatalf("unexpected output: %q / %q", res.Stdout, res.Stderr)
	}
}

func TestRunScriptSuccess(t *testing.T) {
	res := RunScript(context.Background(), protocol.RunScriptPayload{Shell: "sh", Script: "true", TimeoutSeconds: 10})
	if res.Status != protocol.StatusSucceeded {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunScriptTimeoutKillsChildren(t *testing.T) {
	res := RunScript(context.Background(), protocol.RunScriptPayload{Shell: "sh", Script: "sleep 30 & sleep 30", TimeoutSeconds: 1})
	if res.Status != protocol.StatusTimeout {
		t.Fatalf("expected timeout, got %+v", res)
	}
}

func TestRunScriptRejectsUnknownShell(t *testing.T) {
	res := RunScript(context.Background(), protocol.RunScriptPayload{Shell: "fish", Script: "echo"})
	if res.Status != protocol.StatusFailed || res.Error == "" {
		t.Fatalf("expected failure, got %+v", res)
	}
}

func TestCappedBuffer(t *testing.T) {
	var b cappedBuffer
	b.Write([]byte(strings.Repeat("x", protocol.MaxOutputBytes+10)))
	if !strings.HasSuffix(b.String(), "[output truncated]") {
		t.Fatal("expected truncation marker")
	}
}

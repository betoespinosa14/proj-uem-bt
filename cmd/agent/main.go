// Command uem-agent is the endpoint agent. It enrolls the device with the
// UEM server, then runs as a system service (systemd, launchd, or Windows
// service) that reports inventory and executes remote commands.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kardianos/service"

	"github.com/uem/uem/internal/agent/collect"
	"github.com/uem/uem/internal/agent/config"
	"github.com/uem/uem/internal/agent/conn"
	"github.com/uem/uem/internal/protocol"
)

// Version is set at build time with -ldflags "-X main.Version=...".
var Version = "0.1.0-dev"

const usage = `uem-agent — UEM endpoint agent

Usage:
  uem-agent enroll --server URL --token TOKEN   enroll this device
  uem-agent run                                 run in the foreground
  uem-agent install | uninstall                 install/remove the system service
  uem-agent start | stop | status               control the system service
  uem-agent inventory                           print collected inventory as JSON
  uem-agent version

Flags:
  --config PATH   config file (default: ` + "%s" + `)
`

type program struct {
	cancel context.CancelFunc
	done   chan struct{}
	svc    service.Service
	cfg    string
}

func (p *program) Start(s service.Service) error {
	cfg, err := config.Load(p.cfg)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel, p.done = cancel, make(chan struct{})
	a := conn.New(cfg, Version, slog.Default())
	if !service.Interactive() {
		a.OnUninstall = func() { selfUninstall(p.svc, p.cfg) }
	}
	go func() {
		defer close(p.done)
		a.Run(ctx)
	}()
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
		select {
		case <-p.done:
		case <-time.After(10 * time.Second):
		}
	}
	return nil
}

func selfUninstall(s service.Service, cfgPath string) {
	slog.Info("uninstalling agent at server request")
	_ = os.Remove(cfgPath)
	if err := s.Uninstall(); err != nil {
		slog.Error("service uninstall failed", "err", err)
	}
	_ = s.Stop()
	os.Exit(0)
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, usage, config.DefaultPath())
		os.Exit(2)
	}
	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	cfgPath := fs.String("config", config.DefaultPath(), "config file path")
	server := fs.String("server", "", "UEM server URL (enroll)")
	token := fs.String("token", "", "enrollment token (enroll)")
	_ = fs.Parse(os.Args[2:])

	args := []string{"run"}
	if *cfgPath != config.DefaultPath() {
		args = append(args, "--config", *cfgPath)
	}
	prg := &program{cfg: *cfgPath}
	svc, err := service.New(prg, &service.Config{
		Name:        "uem-agent",
		DisplayName: "UEM Agent",
		Description: "Unified endpoint management agent: inventory, compliance, and remote management.",
		Arguments:   args,
		Option:      service.KeyValue{"Restart": "always", "RestartSec": 10, "KeepAlive": true, "RunAtLoad": true, "OnFailure": "restart"},
	})
	if err != nil {
		fatal(err)
	}
	prg.svc = svc

	switch cmd {
	case "enroll":
		if err := enroll(*cfgPath, *server, *token); err != nil {
			fatal(err)
		}
	case "run":
		if err := svc.Run(); err != nil {
			fatal(err)
		}
	case "install", "uninstall", "start", "stop", "restart":
		if err := service.Control(svc, cmd); err != nil {
			fatal(err)
		}
		fmt.Printf("uem-agent: %s ok\n", cmd)
	case "status":
		st, err := svc.Status()
		if err != nil {
			fatal(err)
		}
		names := map[service.Status]string{service.StatusRunning: "running", service.StatusStopped: "stopped"}
		name, ok := names[st]
		if !ok {
			name = "unknown"
		}
		fmt.Println(name)
		if st != service.StatusRunning {
			os.Exit(3)
		}
	case "inventory":
		inv := collect.Inventory(context.Background())
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(inv)
	case "version":
		fmt.Println(Version)
	default:
		fmt.Fprintf(os.Stderr, usage, config.DefaultPath())
		os.Exit(2)
	}
}

func enroll(cfgPath, server, token string) error {
	if server == "" || token == "" {
		return fmt.Errorf("--server and --token are required")
	}
	server = strings.TrimRight(server, "/")
	body, _ := json.Marshal(protocol.EnrollRequest{Token: token, Facts: collect.Facts(context.Background(), Version)})
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Post(server+"/api/agent/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("contacting server: %w", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var e struct{ Error string }
		_ = json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return fmt.Errorf("enrollment failed: %s", e.Error)
	}
	var out protocol.EnrollResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return fmt.Errorf("invalid enrollment response: %w", err)
	}
	if err := config.Save(cfgPath, config.Config{ServerURL: server, DeviceID: out.DeviceID, DeviceSecret: out.DeviceSecret}); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Enrolled as device %s (config: %s)\n", out.DeviceID, cfgPath)
	return nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "uem-agent:", err)
	os.Exit(1)
}

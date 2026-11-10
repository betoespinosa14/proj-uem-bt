package core

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/uem/uem/internal/agentdist"
	"github.com/uem/uem/internal/protocol"
)

// agentUpdate turns an update_agent request into the command for this device: update_agent
// with the build's checksum for agents that support it, or a one-time run_script that
// replaces an older agent (BootstrapScript).
func (c *Core) agentUpdate(ctx context.Context, orgID, deviceID string) (string, json.RawMessage, error) {
	dev, err := c.Store.GetDevice(ctx, orgID, deviceID)
	if err != nil {
		return "", nil, invalid("device not found")
	}
	m, err := agentdist.Load(c.AgentDist)
	if err != nil {
		return "", nil, invalid("no agent build is available on the server")
	}
	file := agentdist.FileName(dev.OSFamily, dev.Arch)
	f, ok := m.Files[file]
	if !ok {
		return "", nil, invalid("no agent build for %s %s", dev.OSFamily, dev.Arch)
	}
	if !agentdist.Outdated(dev.AgentVersion, m.Version) {
		return "", nil, invalid("%s already runs agent %s", dev.Name, dev.AgentVersion)
	}
	up := protocol.UpdatePayload{Version: m.Version, File: file, SHA256: f.SHA256, Size: f.Size}
	if agentdist.SupportsSelfUpdate(dev.AgentVersion) {
		b, _ := json.Marshal(up)
		return protocol.CmdUpdateAgent, b, nil
	}
	org, _ := c.Store.OrgSettingsForDevice(ctx, deviceID)
	base := strings.TrimRight(org.ServerURL, "/")
	if base == "" {
		base = strings.TrimRight(c.PublicURL, "/")
	}
	if base == "" {
		return "", nil, invalid("set the public server URL in Settings first")
	}
	shell, script := BootstrapScript(dev.OSFamily, base+"/downloads/"+file, up)
	b, _ := json.Marshal(map[string]any{"shell": shell, "script": script, "timeout_seconds": 600})
	return protocol.CmdRunScript, b, nil
}

// BootstrapScript replaces an agent that predates update_agent: it downloads the build,
// checks its checksum and that it runs, swaps it in and restarts the service in the
// background (so the script reports its result before the agent restarts).
func BootstrapScript(osFamily, url string, up protocol.UpdatePayload) (shell, script string) {
	r := strings.NewReplacer("{{URL}}", url, "{{SHA}}", up.SHA256, "{{VER}}", up.Version)
	if osFamily == "windows" {
		return "powershell", r.Replace(windowsBootstrap)
	}
	return "sh", r.Replace(unixBootstrap)
}

const unixBootstrap = `# BEACON: one-time update to agent {{VER}}, which can update itself from now on.
set -eu
url='{{URL}}'
sum='{{SHA}}'
bin=/usr/local/bin/uem-agent
sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi; }
if [ "$(sha "$bin")" = "$sum" ]; then
  echo "Agent {{VER}} is already in place; restarting the service onto it."
else
tmp=$(mktemp /usr/local/bin/.uem-agent.XXXXXX)
trap 'rm -f "$tmp"' EXIT
curl -fsSL "$url" -o "$tmp"
[ "$(sha "$tmp")" = "$sum" ] || { echo "Download is damaged (checksum mismatch); the agent was not changed." >&2; exit 1; }
chmod 0755 "$tmp"
v=$("$tmp" version 2>&1 || true)
case "$v" in *'{{VER}}'*) ;; *) echo "The new agent did not run ($v); the agent was not changed." >&2; exit 1 ;; esac
mv -f "$tmp" "$bin"
trap - EXIT
echo "Agent replaced with {{VER}}. It restarts in a few seconds."
fi
# Restart in the background, after this script has reported back.
( sleep 5
  if command -v launchctl >/dev/null 2>&1; then launchctl kickstart -k system/uem-agent
  else systemctl restart uem-agent; fi ) </dev/null >/dev/null 2>&1 &
`

const windowsBootstrap = `# BEACON: one-time update to agent {{VER}}, which can update itself from now on.
$ErrorActionPreference = 'Stop'
$url = '{{URL}}'; $sum = '{{SHA}}'; $ver = '{{VER}}'
$dir = Join-Path $env:ProgramFiles 'UEM Agent'
$bin = Join-Path $dir 'uem-agent.exe'
$new = Join-Path $dir 'uem-agent.new.exe'
if ((Get-FileHash $bin -Algorithm SHA256).Hash -eq $sum.ToUpper()) {
  "Agent $ver is already in place; restarting the service onto it."
} else {
  [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
  Remove-Item $new -Force -ErrorAction SilentlyContinue
  Invoke-WebRequest -UseBasicParsing -Uri $url -OutFile $new
  if ((Get-FileHash $new -Algorithm SHA256).Hash -ne $sum.ToUpper()) { Remove-Item $new -Force; throw 'Download is damaged (checksum mismatch); the agent was not changed.' }
  $v = (& $new version 2>&1) | Out-String
  if ($v -notlike "*$ver*") { Remove-Item $new -Force; throw "The new agent did not run ($v); the agent was not changed." }
  # The running agent can be renamed but not deleted; an older renamed copy may still be in use.
  $old = Join-Path $dir 'uem-agent.old.exe'
  Remove-Item $old -Force -ErrorAction SilentlyContinue
  if (Test-Path $old) { $old = Join-Path $dir ("uem-agent.old-{0}.exe" -f (Get-Date -Format yyyyMMddHHmmss)) }
  Move-Item $bin $old -Force
  Move-Item $new $bin -Force
  "Agent replaced with $ver. It restarts in a few seconds."
}
# Restart from a one-off SYSTEM task, so the restart doesn't depend on this agent staying up.
# Allowed on battery: by default Windows only starts scheduled tasks on mains power.
$task = 'BEACON agent update'
Unregister-ScheduledTask -TaskName $task -Confirm:$false -ErrorAction SilentlyContinue
$action = New-ScheduledTaskAction -Execute 'powershell.exe' -Argument "-NoProfile -Command Start-Sleep 5; Restart-Service uem-agent -Force; Unregister-ScheduledTask -TaskName '$task' -Confirm:` + "`" + `$false"
$settings = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable
Register-ScheduledTask -TaskName $task -Action $action -Settings $settings -User 'SYSTEM' -RunLevel Highest -Force | Out-Null
Start-ScheduledTask -TaskName $task
`

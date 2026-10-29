#!/bin/sh
# UEM agent installer for Linux and macOS.
# Usage: curl -fsSL https://uem.example.com/install.sh | sudo sh -s -- --server https://uem.example.com --token enr_xxx
# Re-running it on an enrolled device upgrades the agent and keeps its identity.
set -eu

SERVER=""
TOKEN=""
while [ $# -gt 0 ]; do
  case "$1" in
    --server) SERVER="$2"; shift 2 ;;
    --token)  TOKEN="$2"; shift 2 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
done
[ -n "$SERVER" ] && [ -n "$TOKEN" ] || { echo "usage: install.sh --server URL --token TOKEN" >&2; exit 2; }
[ "$(id -u)" -eq 0 ] || { echo "install.sh must run as root (use sudo)" >&2; exit 1; }

case "$(uname -s)" in
  Linux)  OS=linux;  CONFIG=/etc/uem-agent/agent.json ;;
  Darwin) OS=darwin; CONFIG="/Library/Application Support/UEM Agent/agent.json" ;;
  *) echo "unsupported OS: $(uname -s)" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64|amd64)  ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

BIN=/usr/local/bin/uem-agent
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT
echo "Downloading UEM agent ($OS/$ARCH)..."
curl -fsSL "$SERVER/downloads/uem-agent-$OS-$ARCH" -o "$TMP"
mkdir -p /usr/local/bin
install -m 0755 "$TMP" "$BIN"

if "$BIN" status >/dev/null 2>&1; then
  "$BIN" stop >/dev/null 2>&1 || true
fi
if [ -f "$CONFIG" ]; then
  echo "Already enrolled: upgrading the agent and keeping this device's identity."
  "$BIN" uninstall >/dev/null 2>&1 || true # re-registered below
else
  echo "Enrolling with $SERVER..."
  "$BIN" enroll --server "$SERVER" --token "$TOKEN"
fi
"$BIN" install
"$BIN" start
echo "UEM agent installed and running."

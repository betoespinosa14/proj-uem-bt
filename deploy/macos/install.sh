#!/bin/bash
# BEACON on the office Mac Mini: run Postgres, the API server, the console and
# Caddy as launchd daemons, so they start at boot without anyone logging in and
# restart if they crash. com.beacon.backup dumps the database nightly at 02:30
# (deploy/macos/backup.sh); it shows "not running" between runs.
#
#   deploy/macos/install.sh            build and stage (run as yourself, no sudo)
#   sudo deploy/macos/install.sh --system
#                                      install/refresh the daemons and restart
#                                      server, console and Caddy on the new build
#
# To redeploy after code changes, run both steps again.
#
# The daemons run as your user, not root, from a copy in
# ~/Library/Application Support/BEACON. They can't run from the repo because
# macOS privacy protection blocks background processes from reading ~/Desktop.
set -euo pipefail

REPO="$(cd "$(dirname "$0")/../.." && pwd)"
LABELS=(postgres server web caddy backup)

if [[ "${1:-}" == "--system" ]]; then
  [[ $EUID -eq 0 ]] || { echo "Run the --system step with sudo." >&2; exit 1; }
  USER_NAME="${SUDO_USER:?run with sudo from your own account}"
else
  [[ $EUID -ne 0 ]] || { echo "Run the build step without sudo." >&2; exit 1; }
  USER_NAME="$(id -un)"
fi
USER_HOME="$(dscl . -read "/Users/$USER_NAME" NFSHomeDirectory | awk '{print $2}')"
APP="$USER_HOME/Library/Application Support/BEACON"
LOGS="$USER_HOME/Library/Logs/BEACON"
RENDERED="$APP/launchd"

stage() {
  echo "==> Building server and agents"
  make -C "$REPO" server build-agents

  echo "==> Building console"
  (cd "$REPO/web" && npm run build)

  echo "==> Staging into $APP"
  mkdir -p "$APP/bin" "$APP/dist" "$APP/web.next" "$LOGS" "$RENDERED"
  # Copy then rename, so a running server keeps its old binary until restarted.
  cp "$REPO/bin/uem-server" "$APP/bin/.uem-server.new"
  mv -f "$APP/bin/.uem-server.new" "$APP/bin/uem-server"
  install -m 755 "$REPO/deploy/macos/backup.sh" "$APP/bin/backup.sh"
  rsync -a --delete "$REPO/dist/" "$APP/dist/"
  # The console is staged beside the running one and swapped in by --system
  # right before the restart: replacing the files under a running console
  # breaks its pages (it keeps serving HTML that names the old files).
  rsync -a --delete "$REPO/web/.next/standalone/" "$APP/web.next/"
  rsync -a --delete "$REPO/web/.next/static/" "$APP/web.next/.next/static/"
  rsync -a --delete "$REPO/web/public/" "$APP/web.next/public/"
  # Remember which static files belong to this build (see swap_web).
  (cd "$APP/web.next/.next/static" && find . -type f | sort) > "$APP/web.next/.build-files"

  # The Caddyfile and server settings are machine config, not code: create
  # them once and leave them alone afterwards.
  if [[ ! -f "$APP/Caddyfile" ]]; then
    cp "$USER_HOME/Library/Application Support/Sentinel/Caddyfile" "$APP/Caddyfile"
    echo "    created $APP/Caddyfile (copied from the Sentinel folder)"
  fi
  if [[ ! -f "$APP/server.env" ]]; then
    umask 077
    cat > "$APP/server.env" <<'EOF'
# BEACON API server settings, read by the com.beacon.server daemon.
# After editing: sudo launchctl kickstart -k system/com.beacon.server
DATABASE_URL=postgres://uem:uem@localhost:5432/uem?sslmode=disable
UEM_AGENT_DIST=dist
UEM_ALLOWED_ORIGINS=localhost:3000
UEM_PUBLIC_URL=http://192.168.130.7:8080
UEM_TRUST_PROXY=true
UEM_SECURE_COOKIES=true
UEM_ALLOW_SIGNUP=false
EOF
    umask 022
    echo "    created $APP/server.env"
  fi

  # Pre-create log files so they belong to you, not root.
  for l in "${LABELS[@]}"; do touch "$LOGS/$l.log"; done

  for l in "${LABELS[@]}"; do
    sed -e "s|__USER__|$USER_NAME|g" -e "s|__HOME__|$USER_HOME|g" \
        -e "s|__APP__|$APP|g" -e "s|__LOGS__|$LOGS|g" \
        "$REPO/deploy/macos/com.beacon.$l.plist" > "$RENDERED/com.beacon.$l.plist"
    plutil -lint -s "$RENDERED/com.beacon.$l.plist"
  done

  echo
  echo "Staged version $(make -s -C "$REPO" version) (agents $(make -s -C "$REPO" agent-version))"
  echo "Next: sudo \"$REPO/deploy/macos/install.sh\" --system"
}

loaded() { launchctl print "system/com.beacon.$1" >/dev/null 2>&1; }

# Days to keep serving the script and style files of earlier console versions.
KEEP_OLD_STATIC_DAYS=14

# Put a staged console in place (the previous one is kept as web.prev).
#
# A browser tab opened before the update still runs the old console, which loads its pages'
# script files by name as you navigate. If those files are gone, its links silently do nothing
# until the tab is reloaded. So the new console also carries the earlier versions' static files
# (they're named by content hash and never clash), and drops them after KEEP_OLD_STATIC_DAYS.
swap_web() {
  [[ -d "$APP/web.next" ]] || return 0
  local new_static="$APP/web.next/.next/static"
  if [[ -d "$APP/web/.next/static" && -f "$APP/web.next/.build-files" ]]; then
    rsync -a --ignore-existing "$APP/web/.next/static/" "$new_static/"
    # The version being replaced starts its countdown now, however long ago it was built.
    if [[ -f "$APP/web/.build-files" ]]; then
      (cd "$new_static" && comm -23 "$APP/web/.build-files" "$APP/web.next/.build-files" |
        while IFS= read -r f; do if [[ -f "$f" ]]; then touch "$f"; fi; done)
    fi
    # Carried files (not part of this build) older than the limit are dropped.
    (cd "$new_static" && find . -type f -mtime +"$KEEP_OLD_STATIC_DAYS" | sort |
      comm -23 - "$APP/web.next/.build-files" | while IFS= read -r f; do rm -f "$f"; done)
    find "$new_static" -type d -empty -delete
    echo "==> Kept earlier console files so open browser tabs keep working"
  fi
  rm -rf "$APP/web.prev"
  if [[ -d "$APP/web" ]]; then mv "$APP/web" "$APP/web.prev"; fi
  mv "$APP/web.next" "$APP/web"
  echo "==> Console updated (previous kept in web.prev)"
}

wait_port_free() {
  for _ in $(seq 1 30); do
    lsof -nP -iTCP:"$1" -sTCP:LISTEN >/dev/null 2>&1 || return 0
    sleep 1
  done
  echo "Port $1 is still in use:" >&2
  lsof -nP -iTCP:"$1" -sTCP:LISTEN >&2
  exit 1
}

install_system() {
  for l in "${LABELS[@]}"; do
    [[ -f "$RENDERED/com.beacon.$l.plist" ]] || { echo "Nothing staged; run the build step first (without sudo)." >&2; exit 1; }
  done

  # First install: stop the hand-started processes and Homebrew's Postgres
  # LaunchAgent (which only runs while someone is logged in).
  if ! loaded caddy; then pkill -f "caddy run --config" || true; fi
  if ! loaded web; then pkill -f "next start" || true; pkill -f "next-server" || true; fi
  if ! loaded server; then pkill -f "go run ./cmd/server" || true; pkill -f "go-build.*/exe/server" || true; fi
  if ! loaded postgres; then
    if [[ -f "$USER_HOME/Library/LaunchAgents/sh.brew.postgresql@16.plist" ]]; then
      echo "==> Stopping Homebrew's Postgres service"
      sudo -u "$USER_NAME" -H /opt/homebrew/bin/brew services stop postgresql@16 || true
    fi
    wait_port_free 5432
  fi

  for l in "${LABELS[@]}"; do
    dst="/Library/LaunchDaemons/com.beacon.$l.plist"
    if [[ $l == web ]]; then swap_web; fi
    if loaded "$l" && cmp -s "$RENDERED/com.beacon.$l.plist" "$dst"; then
      # Unchanged daemon: restart it on the new build (Postgres is left running).
      if [[ $l != postgres ]]; then
        echo "==> Restarting com.beacon.$l"
        launchctl kickstart -k "system/com.beacon.$l"
      fi
      continue
    fi
    if loaded "$l"; then launchctl bootout "system/com.beacon.$l" || true; sleep 1; fi
    case $l in
      server) wait_port_free 8080 ;;
      web) wait_port_free 3000 ;;
      caddy) wait_port_free 443 ;;
    esac
    install -m 644 -o root -g wheel "$RENDERED/com.beacon.$l.plist" "$dst"
    echo "==> Starting com.beacon.$l"
    launchctl bootstrap system "$dst"
    if [[ $l == postgres ]]; then
      for _ in $(seq 1 30); do
        /opt/homebrew/opt/postgresql@16/bin/pg_isready -q -h localhost && break
        sleep 1
      done
    fi
  done

  sleep 3
  echo
  for l in "${LABELS[@]}"; do
    printf '%-20s %s\n' "com.beacon.$l" "$(launchctl print "system/com.beacon.$l" 2>/dev/null | awk '/^\tstate =/{print $3}')"
  done
  echo "Logs: $LOGS"
}

if [[ "${1:-}" == "--system" ]]; then install_system; else stage; fi

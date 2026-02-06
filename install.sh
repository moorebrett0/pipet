#!/usr/bin/env bash
set -euo pipefail

# PiPet installer
# Usage: curl -sSL https://raw.githubusercontent.com/brettsmith/pipet/main/install.sh | sudo bash

REPO="brettsmith/pipet"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/pipet"
DATA_DIR="/var/lib/pipet"
SERVICE_FILE="/etc/systemd/system/pipet.service"

# --- helpers ---

info()  { printf "\033[1;34m→\033[0m %s\n" "$*"; }
ok()    { printf "\033[1;32m✓\033[0m %s\n" "$*"; }
err()   { printf "\033[1;31m✗\033[0m %s\n" "$*" >&2; }
ask()   { printf "\033[1;33m?\033[0m %s " "$1"; read -r "$2"; }

# --- preflight ---

if [ "$(id -u)" -ne 0 ]; then
    err "run as root: curl -sSL ... | sudo bash"
    exit 1
fi

# detect arch
ARCH=$(uname -m)
case "$ARCH" in
    aarch64)       SUFFIX="linux-arm64" ;;
    armv7l|armv6l) SUFFIX="linux-arm"   ;;
    x86_64)        SUFFIX="linux-arm64"; info "x86_64 detected — downloading arm64 binary (for cross-deploy)" ;;
    *)             err "unsupported architecture: $ARCH"; exit 1 ;;
esac

info "PiPet installer"
echo

# --- check for existing install ---

UPGRADING=false
if [ -f "$INSTALL_DIR/pipet" ]; then
    info "existing installation detected"
    UPGRADING=true
fi

# --- download binary ---

info "downloading pipet ($SUFFIX)..."

# Try GitHub releases first, fall back to building from source
RELEASE_URL="https://github.com/$REPO/releases/latest/download/pipet-$SUFFIX"

if curl -fsSL --head "$RELEASE_URL" >/dev/null 2>&1; then
    curl -fsSL "$RELEASE_URL" -o "$INSTALL_DIR/pipet.new"
else
    info "no release binary found — building from source..."

    if ! command -v go >/dev/null 2>&1; then
        err "go is not installed. install with: sudo apt install golang"
        exit 1
    fi

    TMPDIR=$(mktemp -d)
    trap 'rm -rf "$TMPDIR"' EXIT

    info "cloning repo..."
    git clone --depth 1 "https://github.com/$REPO.git" "$TMPDIR/pipet" 2>/dev/null

    info "building..."
    cd "$TMPDIR/pipet"
    CGO_ENABLED=0 go build -ldflags="-s -w" -o "$INSTALL_DIR/pipet.new" ./cmd/pipet
    cd /
fi

chmod 755 "$INSTALL_DIR/pipet.new"
mv "$INSTALL_DIR/pipet.new" "$INSTALL_DIR/pipet"
ok "binary installed to $INSTALL_DIR/pipet"

# --- create user ---

if ! id pipet >/dev/null 2>&1; then
    useradd --system --no-create-home --shell /usr/sbin/nologin pipet
    ok "created pipet user"
fi

# --- directories ---

mkdir -p "$CONFIG_DIR" "$DATA_DIR"
chown pipet:pipet "$DATA_DIR"

# --- config ---

if [ ! -f "$CONFIG_DIR/config.yaml" ]; then
    info "first-time setup — let's configure your pet"
    echo

    ask "Discord bot token:" BOT_TOKEN
    ask "Discord channel ID (where the pet lives):" CHANNEL_ID
    ask "Your Discord user ID (owner):" OWNER_ID

    CLAUDE_KEY=""
    ask "Anthropic API key (optional, press enter to skip):" CLAUDE_KEY

    cat > "$CONFIG_DIR/config.yaml" <<YAML
discord:
  bot_token: "$BOT_TOKEN"
  channel_id: "$CHANNEL_ID"
  owner_ids:
    - "$OWNER_ID"
  allow_spectator_pet: true
  use_threads: true

claude:
  api_key: "$CLAUDE_KEY"
  model: "claude-sonnet-4-5-20250929"
  max_tokens: 1024
  max_tool_iterations: 5
  rate_limit: 10
  rate_window: 1m

pet:
  state_path: "$DATA_DIR/state.json"
  save_interval: 5m

monitor:
  interval: 30s

shell:
  timeout: 10s
  max_output_bytes: 10240

proactive:
  enabled: true
  check_interval: 60s
  morning_hour: 8
  boredom_minutes: 120
  distress_cooldown: 30m
YAML

    chmod 600 "$CONFIG_DIR/config.yaml"
    chown root:pipet "$CONFIG_DIR/config.yaml"
    chmod 640 "$CONFIG_DIR/config.yaml"
    ok "config written to $CONFIG_DIR/config.yaml"
else
    ok "config already exists, keeping it"
fi

# --- systemd ---

cat > "$SERVICE_FILE" <<'SERVICE'
[Unit]
Description=PiPet — digital pet living in Discord
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=/usr/local/bin/pipet -config /etc/pipet/config.yaml
WorkingDirectory=/var/lib/pipet
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
User=pipet
Group=pipet
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/pipet
PrivateTmp=true

[Install]
WantedBy=multi-user.target
SERVICE

systemctl daemon-reload
ok "systemd service installed"

# --- first run: onboarding needs a tty ---

if [ ! -f "$DATA_DIR/state.json" ]; then
    echo
    info "time to hatch your pet!"
    info "running onboarding (this needs your terminal)..."
    echo

    # Run as pipet user but with current tty for stdin
    sudo -u pipet "$INSTALL_DIR/pipet" -config "$CONFIG_DIR/config.yaml" &
    PIPET_PID=$!

    # Wait for state.json to appear (onboarding complete) or process to exit
    for i in $(seq 1 120); do
        if [ -f "$DATA_DIR/state.json" ]; then
            sleep 2
            break
        fi
        if ! kill -0 $PIPET_PID 2>/dev/null; then
            break
        fi
        sleep 1
    done

    # Kill the foreground process — systemd will take over
    kill $PIPET_PID 2>/dev/null || true
    wait $PIPET_PID 2>/dev/null || true
    echo
fi

# --- enable and start ---

systemctl enable pipet >/dev/null 2>&1
if $UPGRADING; then
    systemctl restart pipet
    ok "pipet restarted"
else
    systemctl start pipet
    ok "pipet started"
fi

# --- done ---

echo
ok "pipet is installed and running!"
echo
info "useful commands:"
echo "  journalctl -u pipet -f     # watch logs"
echo "  systemctl status pipet     # check status"
echo "  systemctl restart pipet    # restart"
echo "  sudo nano $CONFIG_DIR/config.yaml  # edit config"
echo

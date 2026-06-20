#!/usr/bin/env bash
set -euo pipefail

# Proxypool Agent installer — public, no secrets embedded.
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/OWNER/proxypool-agent/main/scripts/install.sh | sudo HUB_URL=wss://hub.example.com/tunnel bash
#
# Optional env:
#   HUB_URL          (required) WebSocket tunnel URL
#   GITHUB_REPO      default: doguab/proxypool-agent
#   INSTALL_DIR      default: /usr/local/bin
#   CONFIG_DIR       default: /etc/proxypool
#   VERSION          default: latest

GITHUB_REPO="${GITHUB_REPO:-doguab/proxypool-agent}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
CONFIG_DIR="${CONFIG_DIR:-/etc/proxypool}"
CONFIG_FILE="${CONFIG_DIR}/agent.yaml"
BINARY_NAME="proxypool-agent"
SERVICE_NAME="proxypool-agent"
VERSION="${VERSION:-latest}"

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "Run as root: curl ... | sudo bash" >&2
  exit 1
fi

if [[ -z "${HUB_URL:-}" ]]; then
  echo "HUB_URL is required (e.g. wss://your-hub.example.com/tunnel)" >&2
  exit 1
fi

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
fi

if [[ -z "$VERSION" || "$VERSION" == "null" ]]; then
  echo "Could not resolve release version. Set VERSION manually." >&2
  exit 1
fi

URL="https://github.com/${GITHUB_REPO}/releases/download/${VERSION}/proxypool-agent-linux-${GOARCH}"
TMP="$(mktemp)"
trap 'rm -f "$TMP"' EXIT

echo "Downloading ${URL} ..."
curl -fsSL "$URL" -o "$TMP"
chmod +x "$TMP"
install -m 0755 "$TMP" "${INSTALL_DIR}/${BINARY_NAME}"

mkdir -p "$CONFIG_DIR"

if [[ ! -f "$CONFIG_FILE" ]]; then
  cat >"$CONFIG_FILE" <<EOF
hub_url: ${HUB_URL}
secret: ""
max_connections: 50
EOF
  chmod 600 "$CONFIG_FILE"
else
  # Update hub_url if config exists
  if grep -q '^hub_url:' "$CONFIG_FILE"; then
    sed -i.bak "s|^hub_url:.*|hub_url: ${HUB_URL}|" "$CONFIG_FILE"
    rm -f "${CONFIG_FILE}.bak"
  else
    echo "hub_url: ${HUB_URL}" >>"$CONFIG_FILE"
  fi
fi

UNIT="/etc/systemd/system/${SERVICE_NAME}.service"
cat >"$UNIT" <<EOF
[Unit]
Description=Proxypool Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
ExecStart=${INSTALL_DIR}/${BINARY_NAME} -config ${CONFIG_FILE}
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"

# First run generates secret and prints it
SECRET="$("${INSTALL_DIR}/${BINARY_NAME}" -config "$CONFIG_FILE" -show-secret 2>/dev/null || true)"
if [[ -z "$SECRET" ]]; then
  systemctl restart "$SERVICE_NAME" || true
  sleep 1
  journalctl -u "$SERVICE_NAME" --no-pager -n 30 || true
  SECRET="$("${INSTALL_DIR}/${BINARY_NAME}" -config "$CONFIG_FILE" -show-secret 2>/dev/null || true)"
fi

systemctl restart "$SERVICE_NAME"

echo ""
echo "=================================================="
echo "Proxypool agent installed."
echo "Hub URL: ${HUB_URL}"
if [[ -n "$SECRET" ]]; then
  echo ""
  echo "SAVE THIS SECRET — add it in your hub panel:"
  echo "  ${SECRET}"
fi
echo ""
echo "Status: systemctl status ${SERVICE_NAME}"
echo "=================================================="

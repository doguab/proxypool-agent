# Proxypool Agent

Public, self-hosted reverse-tunnel agent for [Proxypool Hub](https://github.com/doguab/proxypool-hub) (private).

Each agent connects **outbound** to your hub over TLS/WebSocket. No inbound ports are opened on the server. Traffic exits with the server's own public IP.

## Security model

This repository is intentionally **public** and contains **no secrets**:

- No hub credentials, admin tokens, or gateway passwords
- `HUB_URL` is provided at install time (your hub only)
- A per-server `secret` is generated locally on first run
- You register that secret in your private hub panel

Anyone can install this agent, but without a valid secret registered in **your** hub it cannot join your pool.

## Quick install

```bash
curl -fsSL https://raw.githubusercontent.com/doguab/proxypool-agent/main/scripts/install.sh \
  | sudo HUB_URL=wss://proxy.cronwork.com/tunnel bash
```

The installer prints a one-time **secret**. Add it in your hub admin panel.

## Manual config

`/etc/proxypool/agent.yaml`:

```yaml
hub_url: wss://your-hub.example.com/tunnel
secret: <generated-on-first-run>
max_connections: 50
```

```bash
sudo systemctl status proxypool-agent
sudo journalctl -u proxypool-agent -f
```

## Build from source

```bash
go build -o proxypool-agent ./cmd/agent
```

## License

MIT

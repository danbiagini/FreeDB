# FreeDB TUI

Terminal UI for managing app deployments on a FreeDB host.

## Build

Requires Go 1.21+.

```bash
cd tui
make build-linux
```

This produces `build/freedb-linux-amd64`.

## Install

Copy the binary to your FreeDB host:

```bash
scp build/freedb-linux-amd64 your-host:/tmp/freedb
```

On the host:

```bash
sudo cp /tmp/freedb /usr/local/bin/freedb
sudo chmod +x /usr/local/bin/freedb
```

## Run

```bash
sudo freedb
```

The TUI needs root to access the Incus socket.

## Quick Start: Deploy a Test App

[traefik/whoami](https://hub.docker.com/r/traefik/whoami) is a tiny web server that prints request headers — perfect for testing.

1. Run `sudo freedb`
2. Press `a` to add an app
3. Enter the following:
   - **Name:** `whoami`
   - **Image:** `docker.io/traefik/whoami`
   - **Domain:** `whoami.yourdomain.com`
   - **Port:** `80`
   - **Database:** `n`
4. The app deploys and appears on the dashboard
5. Visit `https://whoami.yourdomain.com` (requires DNS pointing to your host's static IP)

## Keyboard Shortcuts

### Dashboard

| Key | Action |
|-----|--------|
| `a` | Add new app |
| `enter` | Manage selected app |
| `r` | Force refresh |
| `↑/↓` | Navigate |
| `q` | Quit |

### Manage App

| Key | Action |
|-----|--------|
| `s` | Stop |
| `t` | Start |
| `r` | Restart |
| `l` | View logs |
| `d` | Delete (with confirmation) |
| `esc` | Back to dashboard |

### Log Viewer

| Key | Action |
|-----|--------|
| `↑/↓` | Scroll |
| `esc` | Back |

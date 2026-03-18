# FreeDB TUI Design

## Overview

A Go + Bubbletea terminal UI that runs on the FreeDB host and provides a single interface for managing app deployments. It abstracts away Incus, Traefik routing, and database provisioning so deploying a new app doesn't require remembering how the platform works.

## Architecture

```
┌─────────────────────────────────────────────┐
│                  TUI (Go)                    │
│                                              │
│  ┌──────────┐ ┌──────────┐ ┌─────────────┐  │
│  │Dashboard │ │ Add App  │ │ Manage App  │  │
│  │  View    │ │ Wizard   │ │ Actions     │  │
│  └────┬─────┘ └────┬─────┘ └──────┬──────┘  │
│       │             │              │         │
│  ┌────┴─────────────┴──────────────┴──────┐  │
│  │          Service Layer                  │  │
│  │  ┌────────┐ ┌────────┐ ┌────────────┐  │  │
│  │  │ Incus  │ │Traefik │ │  Postgres  │  │  │
│  │  │ Client │ │ Routes │ │  Client    │  │  │
│  │  └───┬────┘ └───┬────┘ └─────┬──────┘  │  │
│  │      │          │            │          │  │
│  │  ┌───┴──────────┴────────────┴───────┐  │  │
│  │  │    Cloud Provider Interface       │  │  │
│  │  │  ┌─────────┐  ┌──────────────┐   │  │  │
│  │  │  │  GCP    │  │  AWS (stub)  │   │  │  │
│  │  │  └─────────┘  └──────────────┘   │  │  │
│  │  └───────────────────────────────────┘  │  │
│  └─────────────────────────────────────────┘  │
│                                              │
│  ┌──────────────────────────────────────┐    │
│  │  App Registry (JSON file)            │    │
│  └──────────────────────────────────────┘    │
└─────────────────────────────────────────────┘
         │              │             │
    Incus Socket   proxy1 files    db1:5432
```

## Key Design Decisions

### 1. Incus Go client library, not shelling out

The `github.com/lxc/incus/v6/client` library provides typed access to the Incus API via the local Unix socket. This gives us container lifecycle, stats, exec, and file operations without parsing CLI output. The current scripts shell out to `incus` but the Go client is the canonical integration path.

### 2. Traefik routes via file provider

Traefik watches `/etc/traefik/manual/` inside `proxy1`. To add a route, we render a YAML template and push it via the Incus file API. To remove a route, delete the file. Traefik auto-reloads — no restart needed. The template is embedded in the Go binary via `//go:embed`.

### 3. Direct Postgres connection for DB provisioning

The TUI runs on the host (`10.0.0.1`), and `db1` trusts connections from `10.0.0.1/24`. We connect directly via `lib/pq` to create/drop databases and users — no need to exec into the container.

### 4. IP drift detection, not static IPs

Incus containers get DHCP addresses that can change on restart. Rather than managing static IP allocation, the dashboard detects IP changes on each refresh cycle and regenerates the Traefik route automatically. Simple and matches the existing operational model.

### 5. Two app types: container and VM

Container apps run in Incus on the host. VM apps run on separate cloud instances (for GPU workloads, etc.) and are provisioned via the cloud provider interface. Both get Traefik routes through the same proxy.

## App Registry

JSON file at `/etc/freedb/registry.json`:

```json
{
  "apps": {
    "myapp": {
      "name": "myapp",
      "type": "container",
      "image": "gcr:project/repo/myapp:latest",
      "domain": "myapp.example.com",
      "port": 8080,
      "has_db": true,
      "db_name": "myapp",
      "db_user": "myapp",
      "last_ip": "10.0.0.42",
      "created_at": "2026-03-18T12:00:00Z"
    },
    "ml-pipeline": {
      "name": "ml-pipeline",
      "type": "vm",
      "domain": "ml.example.com",
      "port": 8080,
      "has_db": false,
      "last_ip": "10.0.2.5",
      "cloud_id": "ml-pipeline-vm",
      "vm_spec": {
        "machine_type": "n2-standard-4",
        "disk_size_gb": 100,
        "image": "ubuntu-os-cloud/ubuntu-2404-lts-amd64",
        "zone": "us-central1-a"
      },
      "created_at": "2026-03-18T14:00:00Z"
    }
  }
}
```

Infrastructure containers (`proxy1`, `db1`) appear in the dashboard as "system" entries but are not stored in the registry and cannot be managed through app workflows.

## Cloud Provider Interface

```go
type CloudProvider interface {
    CreateVM(ctx context.Context, name string, spec VMSpec) (cloudID string, err error)
    DeleteVM(ctx context.Context, cloudID string) error
    StartVM(ctx context.Context, cloudID string) error
    StopVM(ctx context.Context, cloudID string) error
    GetVMStatus(ctx context.Context, cloudID string) (VMStatus, error)
    GetVMInternalIP(ctx context.Context, cloudID string) (string, error)
    GetHostInternalIP(ctx context.Context) (string, error)
    UploadBackup(ctx context.Context, localPath, bucket, remotePath string) error
    ProviderName() string
}
```

GCP implements this first using `cloud.google.com/go/compute`. AWS gets a stub that returns "not implemented" errors.

## Traefik Route Template

```yaml
http:
  routers:
    {{.Name}}-router:
      entryPoints:
        - "websecure"
      rule: "Host(`{{.Domain}}`)"
      service: {{.Name}}
      tls:
        certResolver: myresolver
  services:
    {{.Name}}:
      loadBalancer:
        servers:
          - url: "http://{{.IP}}:{{.Port}}/"
```

Same template works for container apps (Incus bridge IP) and VM apps (VPC subnet IP) — only the IP source differs.

## TUI Views

### Dashboard

```
┌─ FreeDB ─────────────────────────────────────────────────────────────┐
│                                                                       │
│  Name          Status    Domain              Mem     Today   7d avg   │
│  ────────────────────────────────────────────────────────────────────│
│  proxy1        RUNNING   —                   64MB    —       —        │
│  db1           RUNNING   —                   256MB   —       —        │
│▸ myapp         RUNNING   myapp.example.com   128MB   47 req  32/day  │
│  ml-pipeline   STOPPED   ml.example.com      —       0       8/day   │
│                                                                       │
│  [a] Add App  [enter] Manage  [q] Quit              Refreshed 2s ago │
└───────────────────────────────────────────────────────────────────────┘
```

- Refreshes every 5 seconds via `tea.Tick`
- Container stats from Incus API (`GetInstanceState`)
- VM stats from cloud provider API (cached, refreshed less frequently)
- Traffic columns ("Today", "7d avg") from Traefik Prometheus metrics + daily snapshots
- System containers (proxy1, db1) shown but not manageable through app workflows

### App Detail View

Shown when selecting an app with Enter. Full traffic breakdown alongside management actions.

```
┌─ myapp ──────────────────────────────────────────────────────────────┐
│                                                                       │
│  Status: RUNNING          Domain: myapp.example.com                   │
│  Type:   container        IP:     10.0.0.42                           │
│  Image:  gcr:proj/myapp   Port:   8080                                │
│  DB:     myapp            Mem:    128MB                                │
│                                                                       │
│  Traffic (last 7 days)                                                │
│  ─────────────────────────────────────────────                        │
│  Today:       47 requests    0 errors                                 │
│  Yesterday:   31 requests    1 error (3.2%)                           │
│  7-day avg:   32 req/day                                              │
│  7-day peak:  89 requests (Mar 14)                                    │
│  Bandwidth:   12.4 MB in / 156 MB out (7d total)                     │
│                                                                       │
│  [s] Stop  [r] Restart  [l] Logs  [d] Delete  [esc] Back             │
└───────────────────────────────────────────────────────────────────────┘
```

### Add App Wizard

Multi-step form using `bubbles/textinput`:

1. **App name** — validated: lowercase alphanumeric + hyphens
2. **App type** — container or vm
3. **Image** — OCI image ref (container) or machine type + OS image (VM)
4. **Domain** — e.g., `myapp.example.com`
5. **Port** — default 8080
6. **Needs database?** — y/n
7. **Confirm** — review all values, deploy on Enter

On confirm, runs the pipeline with a progress spinner:
1. Create container/VM
2. Wait for IP
3. Generate + push Traefik route
4. Create DB if requested (inject `DATABASE_URL` env var)
5. Save to registry

### Manage App

Selected from dashboard via Enter:

```
┌─ myapp ────────────────────────┐
│                                 │
│  [s] Stop                       │
│  [r] Restart                    │
│  [l] View Logs                  │
│  [d] Delete                     │
│  [esc] Back                     │
│                                 │
└─────────────────────────────────┘
```

Delete shows a confirmation dialog and cleans up: container/VM, Traefik route, optional DB drop, registry entry.

## File Structure

```
tui/
  go.mod
  main.go
  internal/
    config/config.go
    registry/
      types.go
      registry.go
    incus/
      client.go
      containers.go
      files.go
    cloud/
      provider.go
      types.go
      gcp/gcp.go
      aws/aws.go          # stub
    traefik/
      routes.go
      template.go          # embedded YAML template
      metrics.go           # scrape and parse Prometheus metrics from proxy1
      history.go           # daily snapshot persistence and 7-day aggregation
    db/
      postgres.go
    tui/
      model.go             # root model, view routing
      dashboard/
        model.go
        view.go
      addapp/
        model.go
        view.go
      manage/
        model.go
        view.go
      components/
        statusbar.go
        confirm.go
        spinner.go
```

## Dependencies

```
github.com/lxc/incus/v6            # Incus Go client
github.com/charmbracelet/bubbletea  # TUI framework
github.com/charmbracelet/bubbles    # Table, textinput, viewport, spinner
github.com/charmbracelet/lipgloss   # Styling
github.com/lib/pq                   # PostgreSQL driver
github.com/prometheus/common        # Prometheus metrics text format parser
cloud.google.com/go/compute         # GCP Compute Engine SDK
golang.org/x/crypto/ssh             # SSH for VM setup
```

## Implementation Phases

### Phase 1: Foundation
- Go module skeleton, Incus client wrapper, app registry, config
- Dashboard view showing all containers with live stats
- **Milestone:** TUI runs on host and displays proxy1, db1, and any app containers

### Phase 2: App Deployment
- Traefik route template rendering and push via Incus file API
- DB provisioning (create user + database via lib/pq)
- Add-app wizard for container type
- IP drift detection on dashboard refresh
- **Milestone:** Deploy a containerized app end-to-end via TUI

### Phase 3: App Management
- Stop/start/restart actions
- Log viewer using bubbles/viewport
- Delete with full cleanup (container, route, DB, registry)
- **Milestone:** Full container app lifecycle through TUI

### Phase 4: Traffic Metrics Dashboard
- Scrape Traefik's Prometheus metrics endpoint (`http://proxy1:8080/metrics`)
- Parse Prometheus text format (lightweight parser or `github.com/prometheus/common/expfmt`)
- Daily snapshot persistence to `/etc/freedb/metrics-history.json`:
  - TUI writes a snapshot on first run each day (or via a lightweight cron)
  - Stores daily totals per app: requests, errors, bytes in/out
  - Retains 30 days, auto-prunes older entries
- Dashboard columns:
  - **Today** — requests since midnight (current counter minus last midnight snapshot)
  - **7d avg** — average daily requests over last 7 days (from snapshot history)
- App detail view (shown on Enter) with full breakdown:
  - Per-day request count and error rate for last 7 days
  - 7-day peak day
  - Total bandwidth in/out
- **Milestone:** Dashboard shows daily traffic KPIs per app; detail view shows 7-day history

### Phase 5: VM Apps + Cloud Provider
- CloudProvider interface with GCP implementation
- Add-app wizard VM path (provision via Compute Engine API)
- VM management (start/stop/delete via cloud API)
- AWS stub
- **Milestone:** Provision a VM-based app via TUI with Traefik routing

### Phase 6: Polish
- Error handling and graceful degradation
- `--check` flag for environment validation
- Makefile with build/install/test targets
- Cross-compilation for linux/amd64

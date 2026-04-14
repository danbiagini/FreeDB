# FreeDB

A complete app hosting platform on a single VM. Deploy containers, manage databases, get automatic HTTPS, and restore from backups — all from a terminal UI or CLI. No YAML files, no Kubernetes, no billing surprises.

```
Internet → Static IP → Traefik (TLS) → App Containers (Incus)
                                      → PostgreSQL
```

## Why not just use Docker?

Docker gives you a container runtime. FreeDB gives you a **hosting platform**:

- **ZFS storage** with snapshots, compression, and data integrity — not overlayfs
- **System containers** alongside OCI containers — run full Ubuntu/Debian VMs for services like Redis
- **Automatic HTTPS** with Let's Encrypt — no nginx configs or cert scripts
- **Database provisioning** — creates the database, user, password, and injects `DATABASE_URL`
- **Per-database backups** with cloud upload and one-command restore
- **Single binary TUI** — SSH in, run `freedb`, press `[a]` to deploy. No compose files
- **Platform upgrades** — versioned migrations that evolve the entire stack

## Why not serverless?

**Fixed-cost hosting.** One VM, predictable billing. No surprises when usage spikes or a browser tab keeps a connection open[^1]. Your apps stay up even when nobody's using them — no cold starts, no free-tier pausing.

**Reproducible setup.** Provision infrastructure with OpenTofu, install with one command, manage with a TUI. Come back after six months and everything still makes sense.

## How It Works

- **[Incus](https://linuxcontainers.org/incus/)** manages lightweight containers on the host VM
- **[Traefik](https://traefik.io/)** handles HTTPS, automatic Let's Encrypt certificates, and routing to apps
- **PostgreSQL** runs in a container with automated nightly backups to cloud storage
- **[Cloud Saver](https://plugins.traefik.io/plugins/673d5ed47dd5a6c3095befdc/cloud-saver)** plugin (optional) auto-shuts down idle VM-based apps to save costs

### Tech Stack

| Layer | Technology |
|---|---|
| Infrastructure | OpenTofu (GCP, AWS) |
| Containers | Incus + ZFS |
| Reverse Proxy | Traefik v3 with automatic TLS |
| Database | PostgreSQL |
| Backups | Nightly per-database pg_dump to cloud storage (30-day retention) |
| App Manager | Go + Bubbletea TUI |

## TUI App Manager

FreeDB includes `freedb`, a terminal UI for managing your deployed apps. SSH in, run `sudo freedb`, and you get a live dashboard with all your containers.

```
FreeDB  test-freedb | GCP | 34.56.78.90 | 2 CPUs

  Name          Status    Image          Domain                Mem     CPU    Reqs   Err%
  ──────────────────────────────────────────────────────────────────────────────────────────
  proxy1        Running   —              —                     38MB    4.3%   —      —
  db1           Running   —              —                     256MB   <0.1%  —      —
  myapp         Running   whoami         myapp.example.com     12MB    <0.1%  47     0
  sportsoil     Running   sportsoil      sportsoil.stage       128MB   1.2%   312    0.1

  Mem: 434 MB  |  CPU: 5.6%  |  Disk: 8.2/50 GB (16%)

  [a] Add App  [enter] Manage  [D] Databases  [R] Registries  [v] Version  [q] Quit
```

### Key Features

- **Deploy apps** from Docker Hub, GCP Artifact Registry, AWS ECR, or any OCI registry
- **System containers** — deploy Ubuntu/Debian system containers for services that don't need external routing
- **Zero-downtime updates** — pull latest image, launch new container, switch Traefik route, then remove old container (blue-green deployment)
- **Architecture detection** — checks OCI image architecture before pulling, shows clear error for mismatches
- **Automatic TLS** — Let's Encrypt certificates provisioned and renewed automatically via Traefik
- **Database management** — create, drop, and list databases from the TUI or CLI
- **Per-database backups** — nightly backup of each database individually with cloud upload
- **Database restore** — restore from backup via TUI or `freedb restore` CLI
- **Environment variable management** — add, edit, delete env vars on running containers
- **Live monitoring** — CPU%, memory, request counts, error rates, and storage pool usage
- **Registry management** — configure private registries with authentication
- **Health checks** — `freedb check` validates the entire platform stack
- **CLI commands** — `freedb list`, `freedb status`, `freedb deploy`, `freedb destroy`, `freedb restore` for scripting and CI/CD
- **Upgrades** — `freedb upgrade` runs versioned migrations with embedded scripts

### Install the TUI

```bash
# Download the latest release (linux/amd64, linux/arm64, or darwin/arm64)
curl -fsSL https://github.com/danbiagini/FreeDB/releases/latest/download/freedb-linux-amd64 \
  -o /usr/local/bin/freedb
chmod +x /usr/local/bin/freedb

# Or build from source
cd tui && make build-linux
scp build/freedb-linux-amd64 host:/usr/local/bin/freedb
```

See [tui/README.md](tui/README.md) for keyboard shortcuts and usage details.

## Architecture

![FreeDB-terraform](https://github.com/user-attachments/assets/9ff95c71-507a-4b22-b406-01e39894c1df)

### Repo Structure

```
infra/          Cloud infrastructure (OpenTofu)
platform/       Host platform setup (Incus, Traefik, PostgreSQL)
apps/           App-specific configs and deploy helpers
ops/            Operational scripts (backups, cron, utilities)
tui/            Terminal UI app manager (Go)
docs/           Design docs
install.sh      One-command bootstrap installer
```

## Quick Start

### 1. Provision infrastructure

FreeDB supports both GCP and AWS. For GCP with OpenTofu:

```bash
cd infra
gcloud storage buckets create gs://freedb-tf-state --location=us-central1 --uniform-bucket-level-access
tofu init
tofu apply -var-file=values.tfvars
```

For AWS, provision an EC2 instance with a Debian 12+ AMI and an attached EBS volume.

### 2. Install the platform

SSH to the host and run the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/danbiagini/FreeDB/main/install.sh | bash
```

The installer handles everything: Incus + ZFS, Traefik, PostgreSQL, backups, registry auth. It reboots once for the ZFS kernel module — just re-run the same command after reboot.

### 3. Deploy apps

Use the TUI:

```bash
sudo freedb
```

Press `[a]` to add an app. The wizard walks through: name, image, domain, port, TLS, database, environment variables.

Or verify the platform is healthy:

```bash
sudo freedb check
```

### Creating a test environment

```bash
cd infra
tofu workspace new test
tofu apply -var-file=test.tfvars
```

See `test.tfvars.example` for required variables.

## Multi-Cloud Support

FreeDB runs on both GCP and AWS with automatic cloud detection:

| Feature | GCP | AWS |
|---|---|---|
| Install | One-command | One-command |
| Private registry | Artifact Registry (auto-configured) | ECR (auto-configured) |
| Auth refresh | 45-min cron (OAuth2 token) | 6-hour cron (ECR token) |
| Backups | Cloud Storage | S3 |

See [docs/multi-cloud-design.md](docs/multi-cloud-design.md) for the full design.

## Backups

PostgreSQL databases are backed up individually every night at 3am via a cron job on the host. Each database gets its own compressed dump file, plus a roles-only dump for user/password recovery. Files are uploaded to cloud storage automatically.

```
/var/lib/freedb/backups/
  roles_20260411_030000Z.sql.gz          # user/role definitions
  mydb_20260411_030000Z.sql.gz           # individual database
  another_20260411_030000Z.sql.gz        # individual database
```

Backups are stored at:
- **Local**: `/var/lib/freedb/backups/` (30-day retention)
- **Cloud**: `gs://BUCKET/HOSTNAME/` (GCP) or `s3://BUCKET/HOSTNAME/` (AWS)

### Configuration

Backup settings are in `/opt/freedb/backup.env`:

```bash
export FREEDB_BACKUP_BUCKET=freedb-backup    # cloud storage bucket name
export FREEDB_DB_CONTAINER=db1               # incus container running PostgreSQL
```

### Manual backup and restore

```bash
# Back up all databases
sudo bash -c '. /opt/freedb/backup.env && /opt/freedb/backup-db.sh'

# Back up a single database
sudo bash -c '. /opt/freedb/backup.env && /opt/freedb/backup-db.sh mydb'

# List available backups for a database
sudo freedb restore mydb

# Restore from a specific backup (drops and recreates the database)
sudo freedb restore mydb 20260411_030000Z
```

Restore is also available in the TUI: press `[D]` for databases, select a database, press `[r]`.

## SSH Tunnel Access

The Traefik dashboard and PostgreSQL are not exposed to the internet. Access them securely via SSH tunnel using stable `.incus` DNS names:

### Traefik Dashboard
```bash
ssh -L 8080:proxy1.incus:8080 user@your-host
# Then open http://localhost:8080 in your browser
```

### Database
```bash
ssh -L 5432:db1.incus:5432 user@your-host
# Then connect locally
psql postgresql://myapp:password@localhost:5432/myapp
```

On GCP via IAP tunnel:
```bash
gcloud compute ssh freedb --zone us-central1-a --tunnel-through-iap -- -L 8080:proxy1.incus:8080
```

## Upgrading

FreeDB supports in-place upgrades via versioned migrations embedded in the binary:

```bash
# Install the new binary (or download from GitHub Releases)
sudo cp freedb-linux-amd64 /usr/local/bin/freedb

# Preview pending migrations
sudo freedb upgrade --dry-run

# Run the upgrade
sudo freedb upgrade

# Retry from a specific version (if a migration failed)
sudo freedb upgrade --from v0.4
```

The upgrade system:
- Tracks the installed version in `/etc/freedb/version`
- Migration scripts are embedded in the binary (no repo clone needed)
- Each migration is idempotent (safe to run multiple times)
- On failure, prints clear retry instructions with `--from`
- Existing installations without a version file are assumed to be v0.2

### Version History

| Version | Changes |
|---|---|
| v0.2 | Initial release with TUI, multi-cloud support |
| v0.3 | Security hardening: HTTPS redirect, PostgreSQL scram-sha-256 auth, Traefik dashboard restricted to SSH tunnel |
| v0.4 | Database management TUI, backup status tracking, configurable Let's Encrypt email |
| v0.5 | Per-database backups, database restore (TUI + CLI), architecture detection, optional Traefik routing, resource summary dashboard, GitHub Releases |

## App Examples

The `apps/` directory contains example app configurations:

- **[CVAT](apps/cvat/)** — Computer Vision Annotation Tool, deployed as a VM-based app with Docker. Includes Terraform example for dedicated GPU instances.

### Let's Encrypt Email

Configure the email used for certificate expiry notifications:

```bash
# Check current email
sudo freedb acme-email

# Set email
sudo freedb acme-email you@example.com
```

## Roadmap

- **Background metrics collector** — systemd timer for daily traffic snapshots, enabling "Today" and "7d avg" columns. See [#6](https://github.com/danbiagini/FreeDB/issues/6).
- **VM-based apps** — provision dedicated cloud VMs for GPU/ML workloads from the TUI.
- **Ephemeral preview environments** — TTL-based containers with auto-cleanup for PR previews.

## References

- [Incus documentation](https://linuxcontainers.org/incus/docs/main/installing/#linux)
- [OpenTofu](https://opentofu.org/)
- [TUI design doc](docs/tui-design.md)
- [Multi-cloud design doc](docs/multi-cloud-design.md)

[^1]: [Streamlit](https://streamlit.io/) offers a nice python-centric development experience with an impressive UX. The FE components use web sockets to communicate with the backend, which means as long as the browser is open there's an active connection keeping the instance up.

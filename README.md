# FreeDB

A self-hosting platform for deploying web applications on a single VM using [Linux Containers](https://linuxcontainers.org/) and a reverse proxy. Provision the infrastructure with OpenTofu, then install the platform with a single command. Manage everything through a terminal UI.

## Why?

I've been doing hobby/side projects for years and keep running into the same problems:

1. **Serverless billing surprises.** Usage is bursty and you exceed quotas, or a user leaves a browser tab open and your Cloud Run instance stays up for days[^1].
2. **Dormant projects rot.** When provisioned with ClickOps, there's no documentation for how the infrastructure works. Coming back after months means re-learning everything.
3. **Free tiers aren't free.** They pause your service when nobody's using it, then it doesn't work the one time someone actually tries it.

FreeDB takes a different approach: a **fixed-cost VM** running a lightweight container platform. You get predictable billing, your apps stay up, and the entire setup is automated and reproducible.

## How It Works

```
Internet → Static IP → Traefik (TLS) → App Containers (Incus)
                                      → PostgreSQL
```

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
| Backups | Nightly pg_dump to cloud storage (30-day retention) |
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

  [a] Add App  [enter] Manage  [R] Registries  [v] Version  [q] Quit
```

### Key Features

- **Deploy apps** from Docker Hub, GCP Artifact Registry, AWS ECR, or any OCI registry
- **Zero-downtime updates** — pull latest image, launch new container, switch Traefik route, then remove old container (blue-green deployment)
- **Automatic TLS** — Let's Encrypt certificates provisioned and renewed automatically via Traefik
- **Database provisioning** — creates PostgreSQL database + user, injects `DATABASE_URL` into the container
- **Environment variable management** — add, edit, delete env vars on running containers
- **Live monitoring** — CPU%, memory, request counts, error rates from Traefik Prometheus metrics
- **Registry management** — configure private registries with authentication
- **Health checks** — `freedb check` validates the entire platform stack

### Install the TUI

```bash
# Download the latest release
curl -fsSL https://github.com/danbiagini/FreeDB/releases/latest/download/freedb-linux-amd64 -o /usr/local/bin/freedb
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

## App Examples

The `apps/` directory contains example app configurations:

- **[CVAT](apps/cvat/)** — Computer Vision Annotation Tool, deployed as a VM-based app with Docker. Includes Terraform example for dedicated GPU instances.

## Roadmap

- **Background metrics collector** — systemd timer for daily traffic snapshots, enabling "Today" and "7d avg" columns. See [#6](https://github.com/danbiagini/FreeDB/issues/6).
- **VM-based apps** — provision dedicated cloud VMs for GPU/ML workloads from the TUI.
- **GitHub Releases** — prebuilt binaries for the TUI without needing Go installed.

## References

- [Incus documentation](https://linuxcontainers.org/incus/docs/main/installing/#linux)
- [OpenTofu](https://opentofu.org/)
- [TUI design doc](docs/tui-design.md)
- [Multi-cloud design doc](docs/multi-cloud-design.md)

[^1]: [Streamlit](https://streamlit.io/) offers a nice python-centric development experience with an impressive UX. The FE components use web sockets to communicate with the backend, which means as long as the browser is open there's an active connection keeping the instance up.

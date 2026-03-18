# FreeDB

A self-hosting platform for deploying web applications on a single VM using [Linux Containers](https://linuxcontainers.org/) and a reverse proxy. Provision the infrastructure with OpenTofu, then install the platform with a single command.

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
                                      → VM Apps (optional, for GPU/ML workloads)
```

- **[Incus](https://linuxcontainers.org/incus/)** manages lightweight containers on the host VM
- **[Traefik](https://traefik.io/)** handles HTTPS, automatic Let's Encrypt certificates, and routing to apps
- **PostgreSQL** runs in a container with automated nightly backups to cloud storage
- **[Cloud Saver](https://plugins.traefik.io/plugins/673d5ed47dd5a6c3095befdc/cloud-saver)** plugin (optional) auto-shuts down idle VM-based apps to save costs

### Tech Stack

| Layer | Technology |
|---|---|
| Infrastructure | OpenTofu (GCP, AWS planned) |
| Containers | Incus + ZFS |
| Reverse Proxy | Traefik v3 with automatic TLS |
| Database | PostgreSQL 15 |
| Backups | Nightly pg_dump to cloud storage (30-day retention) |

## Architecture

![FreeDB-terraform](https://github.com/user-attachments/assets/9ff95c71-507a-4b22-b406-01e39894c1df)

### Repo Structure

```
infra/          Cloud infrastructure (OpenTofu)
platform/       Host platform setup (Incus, Traefik, PostgreSQL)
apps/           App-specific configs and deploy helpers
ops/            Operational scripts (backups, cron, utilities)
docs/           Design docs
install.sh      One-command bootstrap installer
```

## Quick Start

### 1. Provision infrastructure

```bash
cd infra
gcloud storage buckets create gs://freedb-tf-state --location=us-central1 --uniform-bucket-level-access
tofu init
tofu apply -var-file=values.tfvars
```

### 2. Install the platform

SSH to the host and run the installer:

```bash
gcloud compute ssh --zone "us-central1-a" "freedb" --tunnel-through-iap
curl -fsSL https://raw.githubusercontent.com/danbiagini/FreeDB/main/install.sh | bash
```

The installer handles everything: Incus + ZFS, Traefik, PostgreSQL, backups. It reboots once for the ZFS kernel module — just re-run the same command after reboot.

To install a specific version:
```bash
FREEDB_BRANCH=v0.2 curl -fsSL https://raw.githubusercontent.com/danbiagini/FreeDB/main/install.sh | bash
```

### 3. Deploy apps

Deploy container apps using the deploy helper:
```bash
sudo -u incus /home/incus/deploy/deploy-container.sh <name> <remote> <image:tag>
```

### Creating a test environment

```bash
cd infra
tofu workspace new test
tofu apply -var-file=test.tfvars
```

See `test.tfvars.example` for required variables.

### Traefik dashboard

Access via IAP tunnel:
```bash
gcloud compute start-iap-tunnel --zone "us-central1-a" "freedb" 8080
```
Note the local port and connect with web preview on that port.

## App Examples

The `apps/` directory contains example app configurations:

- **[CVAT](apps/cvat/)** — Computer Vision Annotation Tool, deployed as a VM-based app with Docker. Includes Terraform example for dedicated GPU instances.

## Roadmap

- **TUI app manager** — Terminal UI for deploying, managing, and monitoring apps without touching scripts. See [design doc](docs/tui-design.md).
- **Multi-cloud support** — AWS alongside GCP. See [design doc](docs/multi-cloud-design.md).

## References

- [Incus documentation](https://linuxcontainers.org/incus/docs/main/installing/#linux)
- [OpenTofu](https://opentofu.org/)

[^1]: [Streamlit](https://streamlit.io/) offers a nice python-centric development experience with an impressive UX. The FE components use web sockets to communicate with the backend, which means as long as the browser is open there's an active connection keeping the instance up.

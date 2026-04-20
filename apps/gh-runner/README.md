# Self-Hosted GitHub Actions Runner on FreeDB

Deploy apps to FreeDB automatically from GitHub Actions using a self-hosted runner that lives inside the FreeDB network.

## Why a self-hosted runner?

- **No public SSH exposure** — the runner talks to GitHub over outbound HTTPS, and deploys to the host over the internal bridge network (`10.0.0.1`). No need to open SSH to GitHub's IP ranges.
- **Fast image pulls** — the runner is on the same network as the host, so ECR/GCR pulls are fast.
- **No secrets in GitHub** — SSH keys stay inside the FreeDB network. GitHub only needs registry credentials for the image push.

## Architecture

```
GitHub Actions (cloud)
  │
  │  ← outbound HTTPS (runner polls for jobs)
  ▼
┌──────────────────────────────────────────────┐
│ FreeDB Host (10.0.0.1)                       │
│                                              │
│  ┌────────────┐     SSH (10.0.0.1)           │
│  │ gh-runner   │ ──────────────────► freedb   │
│  │ (container) │     deploy myapp             │
│  └────────────┘                              │
│  ┌────────────┐                              │
│  │ myapp       │ ← updated container         │
│  └────────────┘                              │
└──────────────────────────────────────────────┘
```

## Setup

```bash
GITHUB_TOKEN=ghp_xxx GITHUB_REPO=owner/repo sudo ./setup.sh
```

The setup script:
1. Creates an Ubuntu system container (`gh-runner`)
2. Installs the GitHub Actions runner agent
3. Registers with your repository
4. Generates an SSH key and adds it to the host
5. Configures passwordless `sudo freedb deploy` via sudoers

### Options

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_TOKEN` | (required) | GitHub PAT with `repo` scope |
| `GITHUB_REPO` | (required) | `owner/repo` to register with |
| `RUNNER_NAME` | `gh-runner` | Container and runner name |
| `RUNNER_LABELS` | `freedb` | Comma-separated runner labels |
| `RUNNER_USER` | `admin` | Host user for SSH deploys |

## Usage in Workflows

```yaml
deploy:
  runs-on: [self-hosted, freedb]
  steps:
    - name: Deploy
      run: ssh admin@10.0.0.1 "sudo freedb deploy myapp --tag ${{ github.ref_name }} --json"
```

See [deploy-workflow.yml.example](deploy-workflow.yml.example) for a complete workflow with image build, push, and deploy.

## Security

- The runner can **only** run `freedb deploy` as root — the sudoers entry restricts to that exact command
- SSH access is over the internal bridge network (`10.0.0.0/24`), not the public internet
- The runner container has no Traefik route (created with `Expose via Traefik: n`)
- GitHub Actions jobs are isolated — each run gets a clean workspace

## Maintenance

```bash
# Check runner status
sudo incus exec gh-runner -- sudo -u runner bash -c "cd /home/runner && ./svc.sh status"

# View runner logs
sudo incus exec gh-runner -- journalctl -u actions.runner.* -n 50

# Re-register (e.g., after token rotation)
GITHUB_TOKEN=ghp_new GITHUB_REPO=owner/repo sudo ./setup.sh
```

## Removing the runner

```bash
# Deregister from GitHub first
sudo incus exec gh-runner -- sudo -u runner bash -c "cd /home/runner && ./config.sh remove --token TOKEN"

# Then remove the container
sudo freedb destroy gh-runner --yes
```

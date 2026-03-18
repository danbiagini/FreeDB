# Multi-Cloud Support Design

## Goal

Support deploying FreeDB on multiple cloud providers (starting with GCP and AWS) while keeping a single set of platform scripts and app deployment tooling.

## Current State

Everything is GCP-specific:
- `infra/` contains GCP Terraform with GCP provider
- Platform scripts use `gcloud` CLI for metadata, registry auth, backups
- Disk detection uses `/dev/disk/by-id/google-*` paths

## Proposed Architecture

### Infra Layer

Split into per-cloud directories, each self-contained with its own provider, backend, and variables:

```
infra/
  gcp/
    main.tf           # GCP compute, networking, storage
    backend.tf        # GCS backend
    outputs.tf        # Standardized outputs (see below)
    variables.tf
    test.tfvars.example
  aws/
    main.tf           # EC2, VPC, EBS, S3
    backend.tf        # S3 backend
    outputs.tf        # Same standardized outputs
    variables.tf
    test.tfvars.example
```

Both clouds export the same outputs so the platform layer doesn't care which was used:

```hcl
output "host_name"        {}  # instance name
output "external_ip"      {}  # public IP
output "internal_ip"      {}  # private IP
output "backup_bucket"    {}  # bucket/bucket name
output "ssh_command"      {}  # full SSH command for convenience
```

### Resource Mapping

| Concern | GCP | AWS |
|---|---|---|
| Compute | `google_compute_instance` (e2-medium) | `aws_instance` (t3.medium) |
| VPC/Network | Default VPC + `google_compute_subnetwork` | `aws_vpc` + `aws_subnet` + `aws_internet_gateway` + `aws_route_table` |
| Static IP | `google_compute_address` (EXTERNAL) | `aws_eip` + `aws_eip_association` |
| Internal IP | `google_compute_address` (INTERNAL) | Private IP on `aws_network_interface` |
| Persistent Disk | `google_compute_disk` + `attached_disk` | `aws_ebs_volume` + `aws_volume_attachment` |
| Firewall | 4x `google_compute_firewall` | 1x `aws_security_group` with ingress/egress rules |
| Backup Storage | `google_storage_bucket` | `aws_s3_bucket` + `aws_s3_bucket_lifecycle_configuration` |
| SSH Access | IAP tunnel | SSM Session Manager or key pair |
| IAM | Service account + `cloud-platform` scope | `aws_iam_role` + `aws_iam_instance_profile` |
| State Backend | GCS bucket | S3 bucket + DynamoDB for locking |

AWS networking is more verbose (explicit VPC, subnet, internet gateway, route table, NAT gateway) compared to GCP's default network model. This is the biggest difference in Terraform code.

### Platform Layer — Cloud Abstraction

The platform scripts (`incus.sh`, `traefik-instance.sh`, `db-instance.sh`) have a small number of cloud-specific calls. Rather than complex abstractions, use a simple detection + sourcing pattern:

```
platform/
  scripts/
    cloud-env.sh        # Auto-detects cloud, exports functions
    incus.sh             # Sources cloud-env.sh
    traefik-instance.sh
    db-instance.sh
```

`cloud-env.sh` detects which cloud it's running on (via metadata endpoint) and exports:

```bash
# cloud-env.sh

detect_cloud() {
  if curl -sf -m 1 -H "Metadata-Flavor: Google" \
    http://metadata.google.internal/computeMetadata/v1/ >/dev/null 2>&1; then
    echo "gcp"
  elif curl -sf -m 1 http://169.254.169.254/latest/meta-data/ >/dev/null 2>&1; then
    echo "aws"
  else
    echo "unknown"
  fi
}

CLOUD=$(detect_cloud)

case "$CLOUD" in
  gcp)
    get_internal_ip() {
      curl -s -H "Metadata-Flavor: Google" \
        http://metadata.google.internal/computeMetadata/v1/instance/network-interfaces/0/ip
    }
    get_registry_token() {
      curl -s -H "Metadata-Flavor: Google" \
        http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token \
        | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])"
    }
    upload_backup() {
      gcloud storage cp "$1" "gs://$2/$3"
    }
    ;;
  aws)
    TOKEN=$(curl -s -X PUT -H "X-aws-ec2-metadata-token-ttl-seconds: 60" \
      http://169.254.169.254/latest/api/token)
    get_internal_ip() {
      curl -s -H "X-aws-ec2-metadata-token: $TOKEN" \
        http://169.254.169.254/latest/meta-data/local-ipv4
    }
    get_registry_token() {
      aws ecr get-login-password --region us-east-1
    }
    upload_backup() {
      aws s3 cp "$1" "s3://$2/$3"
    }
    ;;
esac
```

### Cloud-Specific Differences in Scripts

| Script | Cloud-specific call | What changes |
|---|---|---|
| `incus.sh` | Registry auth token | GCP metadata token vs AWS ECR token |
| `incus.sh` | Disk detection | `/dev/disk/by-id/google-*` vs `/dev/xvd*` or `/dev/nvme*` |
| `traefik-instance.sh` | Host internal IP | GCP metadata vs AWS IMDS v2 |
| `db-instance.sh` | Host internal IP | Same as above |
| `backup-db.sh` | Upload to cloud storage | `gcloud storage cp` vs `aws s3 cp` |
| `install.sh` | Install cloud CLI | `gcloud` vs `awscli` |

### Disk Detection

This is the trickiest difference. GCP uses predictable paths (`/dev/disk/by-id/google-{name}`), while AWS EBS volumes show up as `/dev/xvdf`, `/dev/nvme1n1`, etc. depending on instance type (Nitro vs non-Nitro).

Approach: detect by size or by process of elimination (find block devices that aren't the root disk):

```bash
# Find non-root block devices
lsblk -dpno NAME,SIZE | grep -v "$(findmnt -n -o SOURCE / | sed 's/[0-9]*$//')" | head -1 | awk '{print $1}'
```

This is cloud-agnostic and would work on both. Could replace the current GCP-specific detection.

## Implementation Order

1. **Extract `cloud-env.sh`** from existing GCP-specific code in the scripts
2. **Refactor scripts** to source `cloud-env.sh` instead of inline cloud calls
3. **Test GCP still works** after refactor
4. **Write `infra/aws/main.tf`** with the resource mapping above
5. **Add AWS case** to `cloud-env.sh`
6. **Test on AWS** end-to-end

Steps 1-3 can be done now without any AWS account. Steps 4-6 need an AWS environment to test against.

## What NOT to Abstract

- Incus, Traefik, PostgreSQL setup — these are OS-level, not cloud-specific
- The TUI — it talks to Incus and the local system, not cloud APIs
- Container image format — OCI images work the same everywhere
- DNS within Incus — purely local networking

## Open Questions

- **Container registry**: GCP Artifact Registry vs AWS ECR have different auth models. The TUI's "add app" flow would need to know which registry to pull from. Could default to Docker Hub to sidestep this.
- **VM-based apps**: The TUI provisioning VMs (for GPU workloads etc.) would need cloud-specific API calls. This could be deferred to a cloud provider plugin in the TUI.
- **Cost optimization**: Cloud-saver plugin is GCP-specific. An AWS equivalent would use EC2 stop/start APIs. This is a cloud-saver plugin concern, not a FreeDB concern.

# FreeDB
Infra as Code for setting up free (as in speech) database, ML pipelines, app stack, etc

https://mihaibojin.medium.com/deploy-and-configure-google-compute-engine-vms-with-terraform-f6b708b226c1


Tried lxd, but then realized I should use Incus
https://linuxcontainers.org/incus/docs/main/installing/#linux

https://docs.rockylinux.org/books/lxd_server/01-install/

# To Run
1. terraform plan -var-file=values.tfvars
1. terraform apply -var-file=values.tfvars

# To Connect

## From cloud shell
gcloud compute ssh --zone "us-central1-a" "freedb" --tunnel-through-iap


## To connect to traefik dashboard using cloud shell tunnel
- gcloud compute start-iap-tunnel --zone "us-central1-a" "freedb" 8080
- Note the local port and connect with web preview on that port

# FreeDB
Infra as Code for setting up free database

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



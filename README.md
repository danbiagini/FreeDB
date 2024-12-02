# FreeDB
Infra as Code for setting up *free* (as in speech) app stack; proxy, database, Computer Vision ML pipeline using terraform & [Linux Containers](https://linuxcontainers.org/) on GCP.

# Rationale (Why)?
I have been doing side / hobby software and tech projects for years, and often chose the "free" version of a platform, or sometimes pay for the "hobby" tier.  This works pretty well, however it's easy to get caught in one of the following situations with having to either accept poor service for the users vs paying a higher bill:
1. Usage is so rare that the project infra / service gets paused, and then doesn't work the "one time" someone is actually trying to use it.
2. Usage is bursty and you end up exceeding the quota for a small period.  For example, with serverless you are left with the choice of paying more, or not meeting the needs of the users.

The other issue is that hobby / side projects are often left dormant for weeks or months, and when provisioned with ClickOps there is no documentation as to what the infrastructure is, how its setup, how to change it, etc.  This can be costly in time; which could be better spent working on the project itself.

# Solution 
FreeDB provisions the following GCP resources using terraform:
1. Two GCP Compute Instances (FreeDB and cvat)
2. A backend subnet for inter instance communication (database, HTTP via proxy)
3. Two external IP addresses (one for HTTPs proxy, the other for outbound internet traffic from cvat)
4. A GCP Cloud Storage bucket for CVAT
5. Two external "google_compute_disk" storage disks (one "standard" for FreeDB, a "balanced" for cvat)
6. A set of FW rules allowing:
 - external HTTPS traffic to FreeDB
 - GCP sourced postgres connections to FreeDB
 - IAP Tunneled traffic to port 8080 on FreeDB


## FreeDB App Hosting Strategy

FreeDB's approach is to use a fixed compute instance as a proxy (with static external IP) and route traffic to either containers running on the host, or to beefier instance(s) for ML workloads.  This has the advantage of being able to turn off the ML instances when not in use, maintaining a static IP address and also can be used to host additional services (i.e. database, small apps, etc).

I've used serverless on a number of platforms (GCP, AWS, Heroku, Azure) and the one thing in common is that you must choose between maintaining a service level vs a fixed cost budget.  As a hobbyist, I prefer fixed budget, but should be able to serve a reasonably high "hobbyist" load.  

My first experience with a serverless budget issue was hosting a [streamlit](https://streamlit.io/) app on Cloud Run and a user left their browser window open on my [SportsOil app](https://app.sportsiol.co) for days[^1].  This caused the Cloud Run instance to remain provisioned "up" until I noticed the budget alert had triggered.  Oops.


## Terraform & GCP Architecture

![FreeDB-terraform](https://github.com/user-attachments/assets/9ff95c71-507a-4b22-b406-01e39894c1df)

# Usage 

## To Run
1. terraform plan -var-file=values.tfvars
1. terraform apply -var-file=values.tfvars

## To Connect

### From cloud shell
gcloud compute ssh --zone "us-central1-a" "freedb" --tunnel-through-iap


### To connect to traefik dashboard using cloud shell tunnel
- gcloud compute start-iap-tunnel --zone "us-central1-a" "freedb" 8080
- Note the local port and connect with web preview on that port

# References
- https://mihaibojin.medium.com/deploy-and-configure-google-compute-engine-vms-with-terraform-f6b708b226c1
- Tried lxd, but then realized I should use [Incus](https://linuxcontainers.org/incus/docs/main/installing/#linux)
- https://docs.rockylinux.org/books/lxd_server/01-install/
[^1]: [Streamlit](https://streamlit.io/) offers a nice python centric development experience and provides an impressive UX with a minimal amount of front end code, great for data intensive apps.  The FE components use web sockets to communicate to the streamlit backend, which provides a responsive and "fast" UX.  It also means that as long as the browser is open there is an active connection with the backend. 

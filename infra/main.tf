# variables
variable "service_account_id" {
 type        = string
 description = "Service account for the EC2 instance"
}

variable "env" {
  type        = string
  description = "Environment name (e.g. 'test', 'staging'). Leave empty for production."
  default     = ""
}

variable "subnet_cidr" {
  type        = string
  description = "CIDR range for the backend subnet"
  default     = "10.0.1.0/24"
}

locals {
  prefix = var.env != "" ? "${var.env}-" : ""
}

resource "google_compute_subnetwork" "default" {
  name          = "${local.prefix}backend-subnet"
  ip_cidr_range = var.subnet_cidr
  region        = "us-central1"
  network       = "default"
}

resource "google_compute_address" "static-ip" {
  provider = google
  name = "${local.prefix}static-ip"
  region = "us-central1"
  address_type = "EXTERNAL"
  network_tier = "STANDARD"
}

resource "google_compute_address" "db-internal-static-ip" {
  name         = "${local.prefix}db-static-internal"
  region       = "us-central1"
  address_type = "INTERNAL"
  subnetwork   = google_compute_subnetwork.default.id
}

data "google_compute_network" "my-network" {
  name = "default"
}

# resource "google_compute_router" "router" {
#    name    = "nat-router"
#    network = data.google_compute_network.my-network.name
#    region  = "us-central1"
# }

# still need to add Cloud NAT service to the router, not supported in terraform yet
# https://cloud.google.com/nat/docs/gce-example#console_5

resource "google_compute_firewall" "default" {
  name    = "${local.prefix}db-firewall"
  network = data.google_compute_network.my-network.name
  priority = 1000
  allow {
    protocol = "tcp"
    ports    = ["5432", "22", "8080"]
  }
  source_ranges = ["35.235.240.0/20"]
  source_tags   = ["${local.prefix}ssh"]
}

resource "google_compute_firewall" "no-rdp-rule" {
  name = "${local.prefix}no-internet-ssh-rdp"
  network = data.google_compute_network.my-network.name
  priority = 2000
  deny {
    protocol = "tcp"
    ports    = ["22","3389"]
  }
  source_ranges = ["0.0.0.0/0"]
}

resource "google_compute_firewall" "web" {
  name    = "${local.prefix}web-internal-firewall"
  network = data.google_compute_network.my-network.name
  priority = 1000
  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }
  source_tags = ["${local.prefix}ssh"]
}

resource "google_compute_firewall" "proxy" {
  name    = "${local.prefix}proxy-firewall"
  network = data.google_compute_network.my-network.name
  priority = 1000
  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }
  source_ranges = ["0.0.0.0/0"]
  target_tags = ["${local.prefix}web"]
}

# Using pd-balanced because it's faster for Compute Engine
resource "google_compute_disk" "data" {
  name = "${local.prefix}freedb-data-1"
  type = "pd-standard"
  zone = "us-central1-a"
  size = "50"
}

resource "google_compute_disk" "cvat-data" {
  name = "${local.prefix}cvat-data-1"
  type = "pd-balanced"
  zone = "us-central1-a"
  size = "50"
}

data "google_service_account" "default" {
  account_id = var.service_account_id
}

# Create a single Compute Engine instance
resource "google_compute_instance" "default" {
  name         = "${local.prefix}freedb"
  machine_type = "e2-medium"
  zone         = "us-central1-a"
  tags         = ["${local.prefix}ssh", "${local.prefix}web"]
  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
      size  = 20
    }
  }

  attached_disk {
    source = google_compute_disk.data.id
    device_name = google_compute_disk.data.name
  }

  metadata_startup_script = "sudo apt update; sudo apt install -yq git"

  network_interface {
    subnetwork = google_compute_subnetwork.default.id
    access_config {
      # Include this section to give the VM an external IP address
      network_tier = "STANDARD"
      nat_ip = google_compute_address.static-ip.address
    }
    network_ip = google_compute_address.db-internal-static-ip.address
  }

  service_account {
    scopes = ["cloud-platform"]
    email = data.google_service_account.default.email
  }
}

resource "google_compute_instance" "freedb-cvat" {
  name         = "${local.prefix}cvat"
  machine_type = "n2-standard-2"
  zone         = "us-central1-a"
  tags         = ["${local.prefix}ssh"]
  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = "ubuntu-os-cloud/ubuntu-2404-lts-amd64"
    }
  }

  attached_disk {
    source = google_compute_disk.cvat-data.id
    device_name = google_compute_disk.cvat-data.name
  }

  network_interface {
    subnetwork = google_compute_subnetwork.default.id
    access_config {
      # Include this section to give the VM an external IP address
      network_tier = "STANDARD"
    }
  }

  service_account {
    scopes = ["cloud-platform"]
    email = data.google_service_account.default.email
  }
}

resource "google_storage_bucket" "static" {
  name          = "${local.prefix}freedb-backup"
  location      = "us-central1"
  storage_class = "STANDARD"

  uniform_bucket_level_access = true
  lifecycle_rule {
    action {
      type = "Delete"
    }
    condition {
      age = 30
      matches_storage_class = ["STANDARD"]
    }
  }
}

output "freedb_external_ip" {
  value = google_compute_address.static-ip.address
}

output "freedb_internal_ip" {
  value = google_compute_address.db-internal-static-ip.address
}

output "freedb_instance_name" {
  value = google_compute_instance.default.name
}

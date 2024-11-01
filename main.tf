# variables
variable "service_account_id" {
 type        = string
 description = "Service account for the EC2 instance"
}

locals {
  force_destroy = "103024"
}

resource "google_compute_subnetwork" "default" {
  name          = "backend-subnet"
  ip_cidr_range = "10.0.1.0/24"
  region        = "us-central1"
  network       = "default"
}

resource "google_compute_address" "static-ip" {
  provider = google
  name = "static-ip"
  region = "us-central1"
  address_type = "EXTERNAL"
  network_tier = "STANDARD"
}

data "google_compute_network" "my-network" {
  name = "default"
}

data "google_compute_address" "db-internal-static-ip" {
  name = "db-static-internal"
  region = "us-central1"
}

resource "google_compute_router" "router" {
  name    = "nat-router"
  network = data.google_compute_network.my-network.name
  region  = "us-central1"
}

# still need to add Cloud NAT service to the router, not supported in terraform yet
# https://cloud.google.com/nat/docs/gce-example#console_5

resource "google_compute_firewall" "default" {
  name    = "db-firewall"
  network = data.google_compute_network.my-network.name
  priority = 1000
  allow {
    protocol = "tcp"
    ports    = ["5432", "22", "8080"]
  }
  source_ranges = ["35.235.240.0/20"]

}

resource "google_compute_firewall" "no-rdp-rule" {
  name = "no-internet-ssh-rdp"
  network = data.google_compute_network.my-network.name
  priority = 2000
  deny {
    protocol = "tcp"
    ports    = ["22","3389"]
  }
  source_ranges = ["0.0.0.0/0"]
}

# Using pd-balanced because it's faster for Compute Engine
resource "google_compute_disk" "data" {
  name = "freedb-data-1"
  type = "pd-standard"
  zone = "us-central1-a"
  size = "50"
}

resource "google_compute_disk" "cvat-data" {
  name = "cvat-data-1"
  type = "pd-balanced"
  zone = "us-central1-a"
  size = "50"
}
data "google_service_account" "default" {
  account_id = var.service_account_id
}

# Create a single Compute Engine instance
resource "google_compute_instance" "default" {
  name         = "freedb"
  machine_type = "e2-medium"
  zone         = "us-central1-a"
  tags         = ["ssh"]
  allow_stopping_for_update = true
  description = "force_destroy ${local.force_destroy}"

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
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
    network_ip = data.google_compute_address.db-internal-static-ip.address
  }

  service_account {
    scopes = ["cloud-platform"]
    email = data.google_service_account.default.email
  }
}

resource "google_compute_instance" "freedb-cvat" {
  name         = "cvat"
  machine_type = "n2-standard-2"
  zone         = "us-central1-a"
  tags         = ["ssh"]
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
  }

  service_account {
    scopes = ["cloud-platform"]
    email = data.google_service_account.default.email
  }
}

resource "google_storage_bucket" "static" {
  name          = "freedb-backup"
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
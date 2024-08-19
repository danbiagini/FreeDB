
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

resource "google_compute_firewall" "default" {
  name    = "db-firewall"
  network = data.google_compute_network.my-network.name

  allow {
    protocol = "tcp"
    ports    = ["5432"]
  }
  source_ranges = ["35.235.240.0/20"]

}

# Create a single Compute Engine instance
resource "google_compute_instance" "default" {
  name         = "freedb"
  machine_type = "e2-small"
  zone         = "us-central1-a"
  tags         = ["ssh"]
  allow_stopping_for_update = true

  boot_disk {
    initialize_params {
      image = "debian-cloud/debian-12"
    }
  }

  metadata_startup_script = "sudo apt-get update; sudo apt-get install -yq incus"
  network_interface {
    subnetwork = google_compute_subnetwork.default.id
    access_config {
      # Include this section to give the VM an external IP address
      network_tier = "STANDARD"
      nat_ip = google_compute_address.static-ip.address
    }
  }
}

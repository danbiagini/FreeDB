# Create a single Compute Engine instance
resource "google_compute_instance" "default" {
  name         = "FreeDB-vm"
  machine_type = "e2-small"
  zone         = "us-central1-a"
  tags         = ["ssh"]

  boot_disk {
    initialize_params {
      image = "rocky-linux-8-optimized-gcp-v20240515"
    }
  }

  network_interface {
    subnetwork = google_compute_subnetwork.default.id

    access_config {
      # Include this section to give the VM an external IP address
    }
  }
}

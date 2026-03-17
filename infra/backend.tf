terraform {
  backend "gcs" {
    bucket = "freedb-tf-state"
    prefix = "terraform/state"
  }
}

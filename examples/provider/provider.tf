terraform {
  required_providers {
    polaris = { source = "ErikBPF/polaris", version = "0.1.0" }
  }
}
provider "polaris" {
  base_url = "https://polaris.example.com"
  realm    = "POLARIS"
  # Supply POLARIS_TOKEN through the environment.
}

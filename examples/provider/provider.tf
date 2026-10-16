terraform {
  required_providers {
    naming = {
      source = "LemurDaniel/naming"
    }
  }
}

provider "naming" {
  # The bundled convention to start from.
  convention = "default"

  # Optional: a YAML file partially overriding it. Every table is merged per
  # key with your value winning, so untouched entries keep the bundled ones.
  # naming = "${path.module}/naming.yaml"

  # Optional: pins <UNIQUE_ID_n> deterministically, so names stay stable
  # across plans and applies without any state.
  random_seed = "myapp-prod"

  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}

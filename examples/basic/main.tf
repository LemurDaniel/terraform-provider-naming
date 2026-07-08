terraform {
  required_providers {
    naming = {
      source = "LemurDaniel/naming"
    }
  }
}

provider "naming" {}

resource "naming_schema" "this" {
  naming     = "${path.module}/naming.yaml"
  parameters = {
    location    = "westeurope"
    environment = "development"
    name        = "myapp"
  }
}

output "abbreviations" {
  value = naming_schema.this
}


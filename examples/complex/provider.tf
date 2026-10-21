# The naming convention is declared once, here. Everything below renders from it.
provider "naming" {
  # The bundled convention to start from.
  convention = "default"

  # A partial override merged over it (see naming.yaml). Untouched entries keep
  # the bundled values.
  naming = "${path.module}/naming.yaml"

  # Pins <UNIQUE_ID_n>, so names stay stable across plans without any state.
  random_seed = "${var.workload}-${var.environment}"

  # Shared by every name. Mapped values (location, environment) are shortened
  # by the convention's mappings, e.g. westeurope -> we.
  parameters = {
    location    = var.location
    environment = var.environment
    name        = var.workload
  }
}

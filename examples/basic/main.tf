
# ----------------------------------------------------------------------------
# Example: basic resource naming with an index range.
#
# Declare the schema once, then generate a full set of sequentially
# named resources in a single naming-generator call.
# ----------------------------------------------------------------------------

module "schema" {
  source = "../.."

  convention = "default"
  naming     = yamldecode(file("${path.module}/naming.basic.yaml"))
  parameters = {
    location    = "westeurope"
    environment = "DEVELOPMENT"
    name        = "TEST"
  }
}
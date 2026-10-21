variable "location" {
  description = "Azure region. Looked up in the convention's location mapping."
  type        = string
  default     = "westeurope"
}

variable "environment" {
  description = "Deployment stage. Looked up in the convention's environment mapping."
  type        = string
  default     = "production"
}

variable "workload" {
  description = "Workload name, rendered into <NAME>."
  type        = string
  default     = "shop"
}

variable "subnets" {
  description = "Subnets to name, keyed by purpose. Each purpose becomes the pattern's optional subname."
  type        = set(string)
  default     = ["web", "app", "data"]
}

variable "vm_count" {
  description = "How many virtual machines to name."
  type        = number
  default     = 3
}

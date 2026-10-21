output "core" {
  value = {
    resource_group  = data.naming_name.resource_group.name
    virtual_network = data.naming_name.virtual_network.name
    key_vault       = data.naming_name.key_vault.name
    redis           = data.naming_name.redis.name
  }
}

output "subnets" {
  value = { for purpose, n in data.naming_name.subnet : purpose => n.name }
}

output "virtual_machines" {
  value = {
    vm         = data.naming_name.virtual_machine.at_index
    os_disks   = data.naming_name.os_disk.at_index
    data_disks = data.naming_name.data_disk.at_index
  }
}

output "variants" {
  value = {
    log_storage       = data.naming_name.log_storage.name
    session_redis     = data.naming_name.session_redis.name
    vm_storage        = data.naming_name.vm_storage.name
    other_region_vnet = data.naming_name.other_region_vnet.name
    aad_group         = data.naming_name.aad_group.name
  }
}

output "schema" {
  value = {
    index_modifier     = data.naming_schema.this.index_modifier
    redis_abbreviation = data.naming_schema.this.abbreviations.Azure["Microsoft.Cache/redis::default"]
    redis_pattern      = data.naming_schema.this.patterns.Azure["Microsoft.Cache/redis"].default
  }
}

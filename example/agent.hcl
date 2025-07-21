# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

# Full configuration options can be found at https://developer.hashicorp.com/nomad/docs/configuration

data_dir  = "/opt/nomad/data"
bind_addr = "0.0.0.0"
plugin_dir = "/opt/nomad/plugins/"

server {
  # license_path is required for Nomad Enterprise as of Nomad v1.1.1+
  #license_path = "/etc/nomad.d/license.hclic"
  enabled          = true
  bootstrap_expect = 1
}

client {
  enabled = true
  servers = ["127.0.0.1"]
}

# Plugin configuration for litegix-fc-driver
plugin "litegix-fc-driver" {
  config {
    vmlinux_path     = "/path/to/vmlinux"
    rootfs_base_path = "/tmp/litegix-rootfs"
  }
}

log_level = "TRACE"

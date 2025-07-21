# Nomad Agent Configuration for Litegix Firecracker Driver
data_dir  = "/opt/nomad/data"
bind_addr = "0.0.0.0"
plugin_dir = "/opt/nomad/plugins/"

server {
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
    vmlinux_path     = "/tmp/dummy-vmlinux"   # Update with real vmlinux path
    rootfs_base_path = "/tmp/litegix-rootfs"
  }
}

log_level = "DEBUG" 
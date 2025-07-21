# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

log_level = "TRACE"

plugin "litegix-fc-driver" {
  config {
    vmlinux_path = "/path/to/vmlinux"
    rootfs_base_path = "/tmp/litegix-rootfs"
  }
}

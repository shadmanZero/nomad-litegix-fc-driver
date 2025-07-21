# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

job "busybox-firecracker" {
  datacenters = ["dc1"]
  type        = "batch"

  group "busybox-group" {
    count = 1

    task "busybox-vm" {
      driver = "litegix-fc-driver"

      config {
        image     = "busybox:latest"
        vpu_count = 1
        mem_size  = 128
        command   = "/bin/sh"
        args      = "-c 'echo \"Hello from BusyBox in Firecracker VM!\"; echo \"Current date: $(date)\"; echo \"System info:\"; uname -a; echo \"Running processes:\"; ps aux; echo \"Sleeping for 30 seconds...\"; sleep 30; echo \"VM task completed!\"'"
        env       = [
          "VM_TYPE=firecracker",
          "CONTAINER_IMAGE=busybox"
        ]
      }

      resources {
        cpu    = 100   # 100 MHz
        memory = 64    # 64 MB (Nomad resources, separate from VM mem_size)
      }

      restart {
        attempts = 2
        interval = "30m"
        delay    = "15s"
        mode     = "fail"
      }
    }
  }
} 
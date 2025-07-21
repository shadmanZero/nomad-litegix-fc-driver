# Copyright (c) HashiCorp, Inc.
# SPDX-License-Identifier: MPL-2.0

job "litegix-example" {
  datacenters = ["dc1"]
  type        = "batch"

  group "example" {
    task "firecracker-vm" {
      driver = "litegix-fc-driver"

      config {
        image = "ubuntu:20.04"
        vpu_count = 1
        mem_size = 512
        command = "/bin/bash"
        args = "-c 'echo Hello from Firecracker VM && sleep 30'"
        env = ["ENV_VAR=value"]
      }

      resources {
        cpu    = 100
        memory = 256
      }
    }
  }
}

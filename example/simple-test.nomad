# Simple test job for Litegix Firecracker Driver
job "simple-firecracker" {
  datacenters = ["dc1"]
  type        = "batch"

  group "test" {
    count = 1

    task "busybox-vm" {
      driver = "litegix-fc-driver"

      config {
        image     = "busybox:latest"
        vpu_count = 1
        mem_size  = 128
        command   = "/bin/sh"
        args      = "-c 'echo \"Hello from Firecracker VM!\"; echo \"Date: $(date)\"; echo \"Hostname: $(hostname)\"; sleep 10; echo \"Task completed\"'"
        env = [
          "VM_TYPE=firecracker",
          "TEST_ENV=simple"
        ]
      }

      resources {
        cpu    = 100
        memory = 64
      }
    }
  }
} 
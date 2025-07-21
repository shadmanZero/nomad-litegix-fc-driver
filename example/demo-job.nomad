# All-in-one Demo Job for Litegix Firecracker Driver
# This job confirms that the VM is running and that 'exec' is working.

job "firecracker-demo" {
  datacenters = ["dc1"]
  type        = "service" # Use 'service' to keep the task running

  group "demo-group" {
    count = 1

    task "live-vm" {
      driver = "litegix-fc-driver"

      config {
        image     = "busybox:latest"
        vpu_count = 1
        mem_size  = 256
        command   = "/bin/sh"
        # This command runs a loop, printing a heartbeat to the logs.
        # This proves the VM is alive and running.
        args      = "-c 'echo \"✅ VM is running and ready for exec testing.\"; while true; do echo \"VM Heartbeat: $(date)\"; sleep 15; done'"
      }

      resources {
        cpu    = 200 # MHz
        memory = 128 # MB
      }
    }
  }
} 
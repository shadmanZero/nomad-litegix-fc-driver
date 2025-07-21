# Simple BusyBox test job for litegix-fc-driver
job "simple-busybox" {
  datacenters = ["dc1"]
  type        = "batch"

  group "test" {
    task "hello" {
      driver = "litegix-fc-driver"

      config {
        image     = "busybox:latest"
        vpu_count = 1
        mem_size  = 64
        args      = "echo 'Hello from Firecracker VM!' && sleep 10"
      }

      resources {
        cpu    = 50
        memory = 32
      }
    }
  }
} 
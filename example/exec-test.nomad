# Long-running job for testing exec functionality
job "exec-test" {
  datacenters = ["dc1"]
  type        = "service"

  group "exec-group" {
    count = 1

    restart {
      attempts = 3
      interval = "5m"
      delay    = "15s"
      mode     = "fail"
    }

    task "exec-vm" {
      driver = "litegix-fc-driver"

      config {
        image     = "busybox:latest"
        vpu_count = 1
        mem_size  = 256
        command   = "/bin/sh"
        args      = "-c 'echo \"=== VM Ready for Exec Testing ===\"; echo \"Started: $(date)\"; echo \"PID: $$\"; while true; do echo \"Heartbeat: $(date)\"; sleep 30; done'"
        env = [
          "VM_TYPE=firecracker-exec",
          "EXEC_ENABLED=true"
        ]
      }

      resources {
        cpu    = 200
        memory = 128
      }

      logs {
        max_files     = 3
        max_file_size = 5
      }
    }
  }
} 
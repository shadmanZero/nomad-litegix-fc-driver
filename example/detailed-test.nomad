# Detailed test job showing VM system information
job "detailed-test" {
  datacenters = ["dc1"]
  type        = "batch"

  group "detailed" {
    count = 1

    task "system-info-vm" {
      driver = "litegix-fc-driver"

      config {
        image     = "busybox:latest"
        vpu_count = 2
        mem_size  = 512
        command   = "/bin/sh"
        args      = <<-EOF
          -c '
          echo "=== Firecracker VM System Information ==="
          echo "Date: $(date)"
          echo "Hostname: $(hostname)"
          echo ""
          echo "=== CPU Information ==="
          cat /proc/cpuinfo | head -20
          echo ""
          echo "=== Memory Information ==="
          cat /proc/meminfo | head -10
          echo ""
          echo "=== Disk Information ==="
          df -h
          echo ""
          echo "=== Network Interfaces ==="
          ip addr show 2>/dev/null || ifconfig
          echo ""
          echo "=== Environment Variables ==="
          env | grep -E "VM_|NOMAD_|PATH|HOME" | sort
          echo ""
          echo "=== Process List ==="
          ps aux
          echo ""
          echo "=== Kernel Version ==="
          uname -a
          echo ""
          echo "=== Uptime ==="
          uptime
          echo ""
          echo "=== Testing for 30 seconds ==="
          sleep 30
          echo "=== VM Test Completed Successfully ==="
          '
        EOF
        env = [
          "VM_TYPE=firecracker-detailed",
          "TEST_MODE=comprehensive",
          "CONTAINER_IMAGE=busybox"
        ]
      }

      resources {
        cpu    = 300
        memory = 256
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
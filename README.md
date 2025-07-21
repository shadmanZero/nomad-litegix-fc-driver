# Nomad Litegix Firecracker Driver

A Nomad driver plugin that uses the Firecracker microVM technology with **full exec support** to run workloads in secure, lightweight virtual machines.

## 🚀 Features

- **Direct Firecracker Integration**: Uses firecracker-go-sdk for VM management
- **OCI Image Support**: Pulls container images and converts to VM rootfs
- **Full Exec Support**: Run commands inside VMs with `nomad alloc exec`
- **VM Agent**: Built-in agent for command execution
- **Resource Management**: Configurable CPU and memory allocation
- **Complete Lifecycle**: Create, start, stop, destroy VMs

## 📋 Requirements

- Linux x86_64 system
- Go 1.24+ (for building)
- Firecracker binary (optional - for real VMs)
- Docker (for OCI image pulling)
- Root privileges for VM operations

## 🛠️ Quick Start

### 1. Build and Test Everything
```bash
# Clone and build
cd nomad-litegix-fc-driver

# Run comprehensive test
chmod +x example/test.sh
./example/test.sh
```

This will:
- ✅ Build the driver
- ✅ Install it to `/opt/nomad/plugins/`
- ✅ Start Nomad with the driver
- ✅ Test basic job execution
- ✅ Test exec functionality

### 2. Manual Setup

If you prefer manual setup:

```bash
# Build driver
go build -o nomad-litegix-fc-driver

# Install driver
sudo cp nomad-litegix-fc-driver /opt/nomad/plugins/
sudo chmod +x /opt/nomad/plugins/nomad-litegix-fc-driver

# Create test vmlinux (for testing)
sudo touch /tmp/dummy-vmlinux

# Start Nomad
sudo nomad agent -dev -config=example/agent.hcl -plugin-dir=/opt/nomad/plugins/
```

## 📄 Job Examples

### Simple Test Job
```bash
nomad job run example/simple-test.nomad
nomad job status simple-firecracker
nomad alloc logs <ALLOC_ID>
```

### Long-running Service (for exec testing)
```bash
nomad job run example/exec-test.nomad
nomad job status exec-test

# Use exec functionality
ALLOC_ID=$(nomad job allocs -json exec-test | jq -r '.[0].ID')
nomad alloc exec $ALLOC_ID hostname
nomad alloc exec $ALLOC_ID ps aux
nomad alloc exec $ALLOC_ID ls -la /
nomad alloc exec -i -t $ALLOC_ID /bin/sh  # Interactive shell
```

### Detailed System Info
```bash
nomad job run example/detailed-test.nomad
nomad alloc logs <ALLOC_ID>
```

## 🔧 Configuration

### Agent Configuration (`agent.hcl`)
```hcl
plugin "litegix-fc-driver" {
  config {
    vmlinux_path     = "/path/to/vmlinux"     # Required: kernel image
    rootfs_base_path = "/tmp/litegix-rootfs"  # Required: rootfs storage
  }
}
```

### Job Configuration
```hcl
task "my-vm" {
  driver = "litegix-fc-driver"
  
  config {
    image     = "busybox:latest"  # Required: OCI image
    vpu_count = 1                 # Required: CPU cores
    mem_size  = 256               # Required: Memory in MB
    command   = "/bin/sh"         # Optional: command
    args      = "-c 'echo hello'" # Optional: arguments
    env       = ["VAR=value"]     # Optional: environment
  }
}
```

## 🎯 Exec Functionality

The driver includes a **VM agent** that enables full exec support:

### Basic Commands
```bash
# Get allocation ID
ALLOC_ID=$(nomad job allocs -json <JOB_NAME> | jq -r '.[0].ID')

# Run commands
nomad alloc exec $ALLOC_ID hostname
nomad alloc exec $ALLOC_ID ps aux
nomad alloc exec $ALLOC_ID cat /proc/meminfo
nomad alloc exec $ALLOC_ID env

# Interactive shell
nomad alloc exec -i -t $ALLOC_ID /bin/sh
```

### How It Works
1. **VM Agent**: Automatically injected into VM rootfs during creation
2. **Communication**: Uses network sockets for command execution
3. **Fallback**: Graceful fallback if agent unavailable
4. **Security**: Commands run with VM isolation

## 📊 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Nomad Agent                            │
├─────────────────────────────────────────────────────────────┤
│                 Litegix FC Driver                           │
│  ┌─────────────────┐  ┌─────────────────────────────────────┤
│  │   OCI Image     │  │        Firecracker VMs              │
│  │   Processing    │  │  ┌──────────┐  ┌──────────┐         │
│  │   + VM Agent    │  │  │   VM 1   │  │   VM 2   │   ...   │
│  │   Injection     │  │  │ + Agent  │  │ + Agent  │         │
│  └─────────────────┘  │  └──────────┘  └──────────┘         │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                     Host System                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────── │
│  │   Docker    │  │ Firecracker │  │   VM Agent Network     │
│  │   Engine    │  │   Binary    │  │   Communication        │
│  └─────────────┘  └─────────────┘  └─────────────────────── │
└─────────────────────────────────────────────────────────────┘
```

## 🔍 Monitoring & Debugging

### Check Driver Status
```bash
nomad node status -self | grep litegix
```

### View Driver Logs
```bash
# Real-time logs
sudo journalctl -u nomad -f | grep litegix

# Recent activity
sudo journalctl -u nomad --since "10 minutes ago" | grep litegix
```

### Job Monitoring
```bash
nomad job status <JOB_NAME>
nomad alloc status <ALLOC_ID>
nomad alloc logs -f <ALLOC_ID>
```

## 🛡️ Security & Isolation

- **VM Isolation**: Each task runs in separate Firecracker microVM
- **Resource Limits**: CPU and memory enforcement at VM level
- **Network Isolation**: Default Firecracker networking
- **Process Isolation**: Complete separation from host system
- **Secure Exec**: Commands execute within VM boundary

## 📂 Files Overview

```
nomad-litegix-fc-driver/
├── litegix/
│   ├── driver.go      # Main driver implementation
│   ├── vm_manager.go  # VM lifecycle management
│   ├── vm_agent.go    # Exec agent implementation
│   ├── handle.go      # Task handle management
│   └── state.go       # Task storage
├── example/
│   ├── agent.hcl           # Nomad agent config
│   ├── simple-test.nomad   # Simple test job
│   ├── exec-test.nomad     # Long-running job for exec
│   ├── detailed-test.nomad # Comprehensive test
│   └── test.sh            # Test runner script
├── main.go           # Driver entry point
└── README.md         # This file
```

## 🎮 Usage Examples

### 1. Quick Test
```bash
./example/test.sh
```

### 2. Development Workflow
```bash
# Build and install
go build -o nomad-litegix-fc-driver
sudo cp nomad-litegix-fc-driver /opt/nomad/plugins/

# Start Nomad
sudo nomad agent -dev -config=example/agent.hcl -plugin-dir=/opt/nomad/plugins/

# Run job
nomad job run example/simple-test.nomad

# Test exec
ALLOC_ID=$(nomad job allocs -json simple-test | jq -r '.[0].ID')
nomad alloc exec $ALLOC_ID /bin/sh
```

### 3. Production Setup
```bash
# Use real vmlinux kernel
# Update agent.hcl with actual vmlinux_path
# Deploy to production Nomad cluster
```

## 🤝 Contributing

1. Fork the repository
2. Create feature branch
3. Make changes
4. Test with `./example/test.sh`
5. Submit pull request

## 📜 License

MPL-2.0 License - see LICENSE file for details.

## 🆘 Support

- **Issues**: Create GitHub issue
- **Documentation**: Check Nomad driver docs
- **Logs**: Use `sudo journalctl -u nomad -f | grep litegix`
- **Exec Problems**: Check VM agent injection in logs

---

🎉 **You now have a complete Firecracker driver with full exec support!** 🎉 
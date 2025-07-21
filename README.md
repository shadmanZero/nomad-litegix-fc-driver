# Nomad Litegix Firecracker Driver

A Nomad driver plugin that uses the Firecracker microVM technology to run workloads in lightweight, secure virtual machines. This driver uses the [firecracker-go-sdk](https://github.com/firecracker-microvm/firecracker-go-sdk) directly to manage VMs instead of containerd.

## Features

- **Direct Firecracker Integration**: Uses firecracker-go-sdk for direct VM management
- **OCI Image Support**: Pulls OCI container images and converts them to VM rootfs
- **Resource Management**: Configurable CPU and memory allocation for VMs
- **Lifecycle Management**: Full start, stop, and destroy VM lifecycle support
- **Automatic Cleanup**: Cleans up VM resources when tasks complete

## Requirements

- Linux x86_64 system
- Firecracker binary installed and accessible
- vmlinux kernel image
- Docker (for OCI image pulling)
- Root privileges or appropriate capabilities for:
  - Creating loop devices
  - Mounting filesystems
  - Managing firecracker VMs

## Installation

1. Build the driver:

```bash
cd nomad-litegix-fc-driver
go build -o nomad-litegix-fc-driver
```

2. Place the binary in Nomad's plugin directory (e.g., `/opt/nomad/plugins/`)

3. Download or build a vmlinux kernel image for Firecracker

## Configuration

### Driver Configuration

Configure the driver in your Nomad agent configuration:

```hcl
plugin "litegix-fc-driver" {
  config {
    vmlinux_path     = "/path/to/vmlinux"
    rootfs_base_path = "/tmp/litegix-rootfs"
  }
}
```

**Configuration Options:**

- `vmlinux_path` (required): Path to the vmlinux kernel image for Firecracker VMs
- `rootfs_base_path` (required): Base directory where VM rootfs images will be stored
- `containerd_socket` (optional): Reserved for future use

### Task Configuration

Define tasks using the litegix-fc-driver:

```hcl
job "firecracker-example" {
  datacenters = ["dc1"]
  type        = "batch"

  group "vm-group" {
    task "ubuntu-vm" {
      driver = "litegix-fc-driver"

      config {
        image     = "ubuntu:20.04"
        vpu_count = 1
        mem_size  = 512
        command   = "/bin/bash"
        args      = "-c 'echo Hello from Firecracker VM && sleep 30'"
        env       = ["ENV_VAR=value", "ANOTHER_VAR=value2"]
      }

      resources {
        cpu    = 100
        memory = 256
      }
    }
  }
}
```

**Task Configuration Options:**

- `image` (required): OCI container image to use as the VM rootfs
- `vpu_count` (required): Number of virtual CPUs for the VM
- `mem_size` (required): Memory size in MB for the VM
- `command` (optional): Command to run inside the VM
- `args` (optional): Arguments for the command
- `env` (optional): Environment variables to set

## How It Works

1. **Image Pulling**: The driver uses Docker to pull OCI container images
2. **Rootfs Creation**: Extracts image layers and creates an ext4 filesystem
3. **VM Creation**: Configures and starts a Firecracker microVM
4. **Monitoring**: Continuously monitors VM status and reports to Nomad
5. **Cleanup**: Automatically cleans up VM resources when tasks complete

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Nomad Agent                            │
├─────────────────────────────────────────────────────────────┤
│                 Litegix FC Driver                           │
├─────────────────────────────────────────────────────────────┤
│                   VM Manager                                │
│  ┌─────────────────┐  ┌─────────────────────────────────────┤
│  │   OCI Image     │  │        Firecracker VMs              │
│  │   Processing    │  │  ┌──────────┐  ┌──────────┐         │
│  │                 │  │  │   VM 1   │  │   VM 2   │   ...   │
│  └─────────────────┘  │  └──────────┘  └──────────┘         │
└─────────────────────────────────────────────────────────────┘
┌─────────────────────────────────────────────────────────────┐
│                     Host System                             │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────── │
│  │   Docker    │  │ Loop Devices│  │   Firecracker Binary   │
│  │   Engine    │  │    & ext4   │  │                        │
│  └─────────────┘  └─────────────┘  └─────────────────────── │
└─────────────────────────────────────────────────────────────┘
```

## Limitations

- **Image Format**: Only supports OCI container images (via Docker)
- **Exec Support**: Does not support `nomad exec` (would require agent inside VM)
- **Stats**: Basic resource statistics only (can be enhanced)
- **Recovery**: Limited recovery support after Nomad agent restarts
- **Networking**: Uses default Firecracker networking (can be enhanced)

## Security Considerations

- VMs provide strong isolation between workloads
- Requires elevated privileges for filesystem and VM operations
- Consider using appropriate seccomp/AppArmor profiles
- Regular security updates for vmlinux kernel images

## Getting a vmlinux Image

You can obtain a vmlinux image through several methods:

1. **Build from source**: Compile a custom Linux kernel with Firecracker-specific configuration
2. **Download pre-built**: Use pre-built images from Firecracker releases
3. **Extract from distribution**: Extract from your distribution's kernel packages

Example configuration for building a minimal kernel:

```bash
# Enable required options in kernel config
CONFIG_VIRTIO=y
CONFIG_VIRTIO_BLK=y
CONFIG_VIRTIO_NET=y
CONFIG_SERIAL_8250=y
CONFIG_SERIAL_8250_CONSOLE=y
```

## Development

### Building

```bash
go mod download
go build -o nomad-litegix-fc-driver
```

### Testing

```bash
# Unit tests
go test ./...

# Integration test with Nomad
nomad agent -dev -config=example/agent.hcl -plugin-dir=.
nomad job run example/example.nomad
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

This project is licensed under the MPL-2.0 License - see the LICENSE file for details.

## Support

For issues and questions:

- Create an issue in the GitHub repository
- Check the Nomad driver development documentation
- Review Firecracker documentation for VM-specific issues

# Example Jobs for Litegix Firecracker Driver

This directory contains example Nomad job files for testing the litegix-fc-driver.

## Files

- `agent.hcl` - Nomad agent configuration with the litegix-fc-driver plugin
- `busybox.nomad` - Comprehensive BusyBox test job with detailed output
- `simple-busybox.nomad` - Minimal BusyBox test job for quick testing
- `example.nomad` - Ubuntu-based example job

## Quick Start

### 1. Start Nomad Agent

```bash
# Make sure the driver binary is in the plugin directory
sudo nomad agent -dev -config=example/agent.hcl -plugin-dir=/opt/nomad/plugins/
```

### 2. Run Simple Test

```bash
# In another terminal
nomad job run example/simple-busybox.nomad
```

### 3. Check Job Status

```bash
nomad job status simple-busybox
nomad alloc logs <ALLOCATION_ID>
```

## Job Examples

### Simple BusyBox (`simple-busybox.nomad`)
- **Image**: `busybox:latest`
- **Memory**: 64MB VM
- **CPU**: 1 vCPU
- **Action**: Prints hello message and sleeps for 10 seconds

### Detailed BusyBox (`busybox.nomad`)
- **Image**: `busybox:latest` 
- **Memory**: 128MB VM
- **CPU**: 1 vCPU
- **Action**: Shows system info, processes, and runs for 30 seconds

## Testing Commands

```bash
# Run the simple test
nomad job run example/simple-busybox.nomad

# Check status
nomad job status simple-busybox

# Get allocation ID
nomad job allocs simple-busybox

# View logs
nomad alloc logs <ALLOCATION_ID>

# Stop the job
nomad job stop simple-busybox

# Run the detailed test
nomad job run example/busybox.nomad
nomad alloc logs $(nomad job allocs -json busybox-firecracker | jq -r '.[0].ID')
```

## Troubleshooting

### Common Issues

1. **Driver not found**: Ensure the binary is in `/opt/nomad/plugins/`
2. **vmlinux missing**: Update `vmlinux_path` in `agent.hcl`
3. **Permission denied**: Run Nomad with sufficient privileges
4. **Docker not available**: Install Docker for OCI image pulling

### Debug Commands

```bash
# Check plugin status
nomad node status -self

# View detailed logs
tail -f /opt/nomad/data/nomad.log

# Check driver capabilities
nomad node status -verbose
```

## Configuration Notes

- **VM Memory** (`mem_size`): Memory allocated to the Firecracker VM
- **Nomad Resources**: Resources allocated by Nomad scheduler (can be different)
- **vpu_count**: Number of virtual CPUs for the VM
- **image**: Any OCI-compatible container image

## Expected Output

For the simple busybox job, you should see:
```
Hello from Firecracker VM!
```

For the detailed busybox job, you should see system information, process list, and timing details. 
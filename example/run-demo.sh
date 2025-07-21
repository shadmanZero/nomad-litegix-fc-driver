#!/bin/bash

# This script provides a full demonstration of the Litegix Firecracker Driver.
# It rebuilds the driver, starts Nomad, runs a long-running job,
# and then tests the 'exec' functionality.

# --- Configuration ---
DRIVER_DIR=$(pwd)
PLUGIN_DIR="/opt/nomad/plugins"
AGENT_CONFIG="$DRIVER_DIR/example/agent.hcl"
DEMO_JOB="$DRIVER_DIR/example/demo-job.nomad"

# --- Helper Functions ---
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m'

info() { echo -e "${BLUE}[INFO]${NC} $1"; }
success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; }

# --- Script Start ---
info "Starting Litegix Firecracker Driver Demo..."

# 1. Stop any previous Nomad instance
info "Stopping any running Nomad instance..."
sudo pkill nomad

# 2. Rebuild the driver
info "Rebuilding the driver to ensure it's up-to-date..."
go build -o nomad-litegix-fc-driver
if [ $? -ne 0 ]; then
    error "Driver build failed!"
    exit 1
fi
success "Driver built successfully."

# 3. Install the driver
info "Installing the driver to $PLUGIN_DIR..."
sudo mkdir -p $PLUGIN_DIR
sudo cp nomad-litegix-fc-driver $PLUGIN_DIR/
sudo chmod +x $PLUGIN_DIR/nomad-litegix-fc-driver
success "Driver installed."

# 4. Start Nomad in the background
info "Starting Nomad agent in the background..."
sudo nomad agent -dev -config=$AGENT_CONFIG -plugin-dir=$PLUGIN_DIR &> /tmp/nomad-demo.log &
NOMAD_PID=$!
sleep 5 # Give Nomad time to start

# Verify Nomad started
if ! ps -p $NOMAD_PID > /dev/null; then
    error "Nomad agent failed to start. Check /tmp/nomad-demo.log for details."
    exit 1
fi
success "Nomad agent is running (PID: $NOMAD_PID)."

# 5. Check driver health
info "Verifying the driver is loaded and healthy..."
if ! nomad node status -self -verbose | grep -q "litegix-fc-driver.*true.*true"; then
    error "Driver is not healthy or does not support exec. Check Nomad logs."
    sudo pkill nomad
    exit 1
fi
success "Driver is healthy and exec is enabled!"

# 6. Run the demo job
info "Submitting the demo job (example/demo-job.nomad)..."
nomad job run $DEMO_JOB
sleep 10 # Allow time for the job to be scheduled and start

# 7. Confirm VM is running
info "Confirming the VM is running by checking logs for a heartbeat..."
if nomad alloc logs $(nomad job allocs -q firecracker-demo) | grep -q "VM Heartbeat"; then
    success "VM is confirmed running!"
else
    error "VM does not appear to be running. Check allocation logs."
    sudo pkill nomad
    exit 1
fi

# 8. Test Exec Functionality
info "Testing exec functionality..."
ALLOC_ID=$(nomad job allocs -q firecracker-demo)

echo ""
info "--- Running 'hostname' inside the VM ---"
nomad alloc exec $ALLOC_ID hostname
echo "----------------------------------------"

echo ""
info "--- Running 'ps aux' inside the VM ---"
nomad alloc exec $ALLOC_ID ps aux
echo "--------------------------------------"

echo ""
info "--- Running 'uname -a' inside the VM ---"
nomad alloc exec $ALLOC_ID uname -a
echo "----------------------------------------"
echo ""

success "Exec tests completed successfully!"

# --- Cleanup ---
info "Demo finished. You can now interact with the running job."
info "To get an interactive shell, run: nomad alloc exec -i -t $ALLOC_ID /bin/sh"
info "To view logs, run: nomad alloc logs -f $ALLOC_ID"
info "To stop the job, run: nomad job stop firecracker-demo"
info "To stop the Nomad agent, run: sudo kill $NOMAD_PID" 
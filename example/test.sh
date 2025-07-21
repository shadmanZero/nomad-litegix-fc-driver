#!/bin/bash

echo "=== Litegix Firecracker Driver Test Script ==="

# Colors
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

print_info() { echo -e "${BLUE}[INFO]${NC} $1"; }
print_success() { echo -e "${GREEN}[SUCCESS]${NC} $1"; }
print_error() { echo -e "${RED}[ERROR]${NC} $1"; }
print_warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }

# Check if we're in the right directory
if [ ! -f "go.mod" ] || [ ! -d "litegix" ]; then
    print_error "Please run this script from the nomad-litegix-fc-driver directory"
    exit 1
fi

print_info "Building and testing Litegix Firecracker Driver..."

# Step 1: Build driver
print_info "Building driver..."
go build -o nomad-litegix-fc-driver
if [ $? -ne 0 ]; then
    print_error "Failed to build driver"
    exit 1
fi
print_success "Driver built successfully"

# Step 2: Install driver
print_info "Installing driver..."
sudo mkdir -p /opt/nomad/plugins/
sudo cp nomad-litegix-fc-driver /opt/nomad/plugins/
sudo chmod +x /opt/nomad/plugins/nomad-litegix-fc-driver
print_success "Driver installed"

# Step 3: Setup test environment
print_info "Setting up test environment..."
sudo touch /tmp/dummy-vmlinux
sudo mkdir -p /opt/nomad/data
print_success "Test environment ready"

# Step 4: Stop any existing Nomad
print_info "Stopping any existing Nomad instances..."
sudo pkill nomad 2>/dev/null || true
sleep 2

# Step 5: Start Nomad
print_info "Starting Nomad agent..."
sudo nomad agent -dev -config=example/agent.hcl -plugin-dir=/opt/nomad/plugins/ > /tmp/nomad-test.log 2>&1 &
NOMAD_PID=$!
print_info "Nomad started (PID: $NOMAD_PID)"

# Step 6: Wait for startup
print_info "Waiting for Nomad to start..."
sleep 8

# Step 7: Check driver status
print_info "Checking driver status..."
for i in {1..30}; do
    if nomad node status -self 2>/dev/null | grep -q "litegix-fc-driver.*healthy"; then
        print_success "Driver is healthy!"
        break
    fi
    sleep 1
done

if ! nomad node status -self 2>/dev/null | grep -q "litegix-fc-driver.*healthy"; then
    print_error "Driver not healthy. Check logs:"
    tail -20 /tmp/nomad-test.log
    kill $NOMAD_PID 2>/dev/null
    exit 1
fi

# Step 8: Test jobs
echo ""
print_info "=== Testing Jobs ==="

# Test 1: Simple job
print_info "Test 1: Running simple job..."
if nomad job run example/simple-test.nomad; then
    print_success "Simple job submitted"
    sleep 5
    nomad job status simple-firecracker
    
    # Get logs
    ALLOC_ID=$(nomad job allocs -json simple-firecracker 2>/dev/null | jq -r '.[0].ID')
    if [ -n "$ALLOC_ID" ] && [ "$ALLOC_ID" != "null" ]; then
        print_info "Job logs:"
        nomad alloc logs "$ALLOC_ID" 2>/dev/null || print_warning "Logs not available yet"
    fi
    
    nomad job stop -purge simple-firecracker 2>/dev/null
else
    print_error "Failed to submit simple job"
fi

echo ""

# Test 2: Exec test job
print_info "Test 2: Running exec test job..."
if nomad job run example/exec-test.nomad; then
    print_success "Exec test job submitted"
    sleep 10
    
    EXEC_ALLOC_ID=$(nomad job allocs -json exec-test 2>/dev/null | jq -r '.[0].ID')
    if [ -n "$EXEC_ALLOC_ID" ] && [ "$EXEC_ALLOC_ID" != "null" ]; then
        print_info "Testing exec functionality..."
        
        # Test basic commands
        print_info "Running: hostname"
        nomad alloc exec "$EXEC_ALLOC_ID" hostname
        
        print_info "Running: ps aux"
        nomad alloc exec "$EXEC_ALLOC_ID" ps aux
        
        print_info "Running: env"
        nomad alloc exec "$EXEC_ALLOC_ID" env | grep -E "VM_|PATH"
        
        print_success "Exec tests completed!"
    fi
    
    nomad job stop -purge exec-test 2>/dev/null
else
    print_error "Failed to submit exec test job"
fi

echo ""

# Summary
print_info "=== Test Summary ==="
print_success "Litegix Firecracker Driver test completed!"
print_info ""
print_info "Available job files:"
echo "  example/simple-test.nomad   - Simple test job"
echo "  example/exec-test.nomad     - Long-running job for exec testing"
echo "  example/detailed-test.nomad - Comprehensive system info job"
print_info ""
print_info "Usage:"
echo "  nomad job run example/simple-test.nomad"
echo "  nomad alloc exec <ALLOC_ID> <command>"
echo "  nomad alloc logs <ALLOC_ID>"
print_info ""
print_info "Nomad is running (PID: $NOMAD_PID)"
print_info "To stop: kill $NOMAD_PID"
print_info "Logs: tail -f /tmp/nomad-test.log" 
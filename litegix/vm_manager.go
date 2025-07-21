package litegix

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	"github.com/hashicorp/go-hclog"
)

const (
	defaultTimeout = 30 * time.Second

	// VM State constants for Nomad compatibility
	VMStateCreated  = "created"
	VMStateRunning  = "running"
	VMStateStopped  = "stopped"
	VMStatePaused   = "paused"
	VMStateUnknown  = "unknown"
	VMStateDeleted  = "deleted"
)

type VMManager interface {
	CreateAndStartVM(ctx context.Context, config *TaskConfig, taskID string) (*VMInfo, error)
	StopVM(ctx context.Context, vmInfo *VMInfo, timeout time.Duration) error
	DestroyVM(ctx context.Context, vmInfo *VMInfo) error
	GetVMStatus(ctx context.Context, vmInfo *VMInfo) (*VMStatus, error)
}

type VMStatus struct {
	State    string
	PID      uint32
	ExitCode *uint32
	ExitedAt *time.Time
	Error    error
}

type VMInfo struct {
	TaskID      string
	VMID        string
	Machine     *firecracker.Machine
	SocketPath  string
	RootfsPath  string
	PID         uint32
	CreatedAt   time.Time
}

type firecrackerVMManager struct {
	config *Config
	logger hclog.Logger
}

// NewVMManager creates a new VM manager instance
func NewVMManager(config *Config, logger hclog.Logger) VMManager {
	return &firecrackerVMManager{
		config: config,
		logger: logger.Named("vm_manager"),
	}
}

// OCI image manifest structure for pulling images
type OCIManifest struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"config"`
	Layers []struct {
		MediaType string `json:"mediaType"`
		Size      int64  `json:"size"`
		Digest    string `json:"digest"`
	} `json:"layers"`
}

func (vm *firecrackerVMManager) pullOCIImage(ctx context.Context, imageName, targetDir string) error {
	logger := vm.logger.With("image", imageName, "target_dir", targetDir)
	
	// For simplicity, we'll use Docker to pull the image and extract it
	// In production, you might want to use a proper OCI image library
	logger.Info("pulling OCI image using Docker")
	
	// Pull the image using Docker
	cmd := exec.CommandContext(ctx, "docker", "pull", imageName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to pull image with docker: %w", err)
	}
	
	// Create the target directory
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}
	
	// Export the image to a tar file
	tarPath := filepath.Join(targetDir, "image.tar")
	cmd = exec.CommandContext(ctx, "docker", "save", "-o", tarPath, imageName)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to save image: %w", err)
	}
	
	// Extract the tar file
	cmd = exec.CommandContext(ctx, "tar", "-xf", tarPath, "-C", targetDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to extract image tar: %w", err)
	}
	
	// Remove the tar file
	os.Remove(tarPath)
	
	logger.Info("successfully pulled and extracted OCI image")
	return nil
}

func (vm *firecrackerVMManager) createRootfs(ctx context.Context, imageDir, rootfsPath string) error {
	logger := vm.logger.With("image_dir", imageDir, "rootfs_path", rootfsPath)
	
	// Read the manifest to understand the image structure
	manifestPath := filepath.Join(imageDir, "manifest.json")
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}
	
	var manifests []struct {
		Config   string   `json:"Config"`
		RepoTags []string `json:"RepoTags"`
		Layers   []string `json:"Layers"`
	}
	
	if err := json.Unmarshal(manifestData, &manifests); err != nil {
		return fmt.Errorf("failed to parse manifest: %w", err)
	}
	
	if len(manifests) == 0 {
		return fmt.Errorf("no manifests found in image")
	}
	
	manifest := manifests[0]
	
	// Create a temporary directory for building the rootfs
	tempDir, err := os.MkdirTemp("", "rootfs-build-")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)
	
	logger.Info("extracting image layers", "layer_count", len(manifest.Layers))
	
	// Extract all layers in order
	for i, layer := range manifest.Layers {
		layerPath := filepath.Join(imageDir, layer)
		logger.Debug("extracting layer", "layer", i+1, "path", layerPath)
		
		cmd := exec.CommandContext(ctx, "tar", "-xf", layerPath, "-C", tempDir)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to extract layer %s: %w", layer, err)
		}
	}
	
	// Create an ext4 filesystem image
	logger.Info("creating ext4 filesystem image")
	
	// Calculate size (add some buffer space)
	cmd := exec.CommandContext(ctx, "du", "-sb", tempDir)
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to calculate directory size: %w", err)
	}
	
	sizeStr := strings.Fields(string(output))[0]
	size, err := strconv.ParseInt(sizeStr, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to parse size: %w", err)
	}
	
	// Add 50% buffer and round up to MB
	sizeWithBuffer := size * 3 / 2
	sizeMB := (sizeWithBuffer + 1024*1024 - 1) / (1024 * 1024)
	if sizeMB < 100 {
		sizeMB = 100 // Minimum 100MB
	}
	
	// Create empty file
	cmd = exec.CommandContext(ctx, "dd", "if=/dev/zero", "of="+rootfsPath, 
		"bs=1M", "count="+strconv.FormatInt(sizeMB, 10))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create rootfs file: %w", err)
	}
	
	// Format as ext4
	cmd = exec.CommandContext(ctx, "mkfs.ext4", "-F", rootfsPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to format rootfs: %w", err)
	}
	
	// Mount the filesystem
	mountDir, err := os.MkdirTemp("", "rootfs-mount-")
	if err != nil {
		return fmt.Errorf("failed to create mount dir: %w", err)
	}
	defer os.RemoveAll(mountDir)
	
	cmd = exec.CommandContext(ctx, "mount", "-o", "loop", rootfsPath, mountDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to mount rootfs: %w", err)
	}
	defer exec.CommandContext(ctx, "umount", mountDir).Run()
	
	// Copy the extracted filesystem to the mounted image
	cmd = exec.CommandContext(ctx, "cp", "-a", tempDir+"/.", mountDir+"/")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy filesystem: %w", err)
	}
	
	logger.Info("successfully created rootfs", "size_mb", sizeMB)
	return nil
}

func (vm *firecrackerVMManager) CreateAndStartVM(ctx context.Context, config *TaskConfig, taskID string) (*VMInfo, error) {
	logger := vm.logger.With("task_id", taskID, "image", config.Image)
	
	// Create directories for this VM
	vmDir := filepath.Join(vm.config.RootfsBasePath, taskID)
	if err := os.MkdirAll(vmDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create VM directory: %w", err)
	}
	
	imageDir := filepath.Join(vmDir, "image")
	rootfsPath := filepath.Join(vmDir, "rootfs.ext4")
	socketPath := filepath.Join(vmDir, "firecracker.sock")
	
	// Pull and extract the OCI image
	logger.Info("pulling OCI image")
	if err := vm.pullOCIImage(ctx, config.Image, imageDir); err != nil {
		return nil, fmt.Errorf("failed to pull OCI image: %w", err)
	}
	
	// Create rootfs from the OCI image
	logger.Info("creating rootfs from OCI image")
	if err := vm.createRootfs(ctx, imageDir, rootfsPath); err != nil {
		return nil, fmt.Errorf("failed to create rootfs: %w", err)
	}
	
	// Prepare firecracker configuration
	logger.Info("configuring firecracker VM")
	
	// Configure drives
	drives := []models.Drive{
		{
			DriveID:      firecracker.String("rootfs"),
			PathOnHost:   &rootfsPath,
			IsRootDevice: firecracker.Bool(true),
			IsReadOnly:   firecracker.Bool(false),
		},
	}
	
	// Configure machine
	machineConfig := models.MachineConfiguration{
		VcpuCount:  firecracker.Int64(int64(config.VpuCount)),
		MemSizeMib: firecracker.Int64(int64(config.MemSize)),
	}
	
	// Create firecracker machine configuration
	fcConfig := firecracker.Config{
		SocketPath:      socketPath,
		KernelImagePath: vm.config.VmlinuxPath,
		KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off",
		Drives:          drives,
		MachineCfg:      machineConfig,
	}
	
	// Create and start the VM
	logger.Info("starting firecracker VM")
	machine, err := firecracker.NewMachine(ctx, fcConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create firecracker machine: %w", err)
	}
	
	if err := machine.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start firecracker VM: %w", err)
	}
	
	// Get the PID
	pid, err := machine.PID()
	if err != nil {
		return nil, fmt.Errorf("failed to get VM PID: %w", err)
	}
	
	logger.Info("VM started successfully", "vm_id", taskID, "pid", pid)
	
	return &VMInfo{
		TaskID:     taskID,
		VMID:       taskID,
		Machine:    machine,
		SocketPath: socketPath,
		RootfsPath: rootfsPath,
		PID:        uint32(pid),
		CreatedAt:  time.Now(),
	}, nil
}

func (vm *firecrackerVMManager) StopVM(ctx context.Context, vmInfo *VMInfo, timeout time.Duration) error {
	logger := vm.logger.With("task_id", vmInfo.TaskID, "vm_id", vmInfo.VMID)
	
	logger.Info("stopping VM", "timeout", timeout)
	
	stopCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	
	if err := vmInfo.Machine.Shutdown(stopCtx); err != nil {
		logger.Warn("failed to shutdown gracefully, stopping forcefully", "error", err)
		return vmInfo.Machine.StopVMM()
	}
	
	logger.Info("VM stopped successfully")
	return nil
}

func (vm *firecrackerVMManager) DestroyVM(ctx context.Context, vmInfo *VMInfo) error {
	logger := vm.logger.With("task_id", vmInfo.TaskID, "vm_id", vmInfo.VMID)
	
	logger.Info("destroying VM")
	
	// Stop the VM if it's still running
	if vmInfo.Machine != nil {
		vmInfo.Machine.StopVMM()
	}
	
	// Clean up VM directory
	vmDir := filepath.Dir(vmInfo.RootfsPath)
	if err := os.RemoveAll(vmDir); err != nil {
		logger.Warn("failed to clean up VM directory", "error", err, "dir", vmDir)
		return fmt.Errorf("failed to clean up VM directory: %w", err)
	}
	
	logger.Info("VM destroyed successfully")
	return nil
}

func (vm *firecrackerVMManager) GetVMStatus(ctx context.Context, vmInfo *VMInfo) (*VMStatus, error) {
	logger := vm.logger.With("task_id", vmInfo.TaskID, "vm_id", vmInfo.VMID)
	
	if vmInfo.Machine == nil {
		logger.Warn("machine is nil, returning unknown status")
		return &VMStatus{State: VMStateUnknown}, nil
	}
	
	// Check if the process is still running
	pid, err := vmInfo.Machine.PID()
	if err != nil || pid == 0 {
		return &VMStatus{
			State: VMStateStopped,
			PID:   vmInfo.PID,
		}, nil
	}
	
	// Try to check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return &VMStatus{
			State: VMStateStopped,
			PID:   vmInfo.PID,
		}, nil
	}
	
	// Try to signal the process to check if it's alive (signal 0 means "check if alive")
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return &VMStatus{
			State: VMStateStopped,
			PID:   vmInfo.PID,
		}, nil
	}
	
	return &VMStatus{
		State: VMStateRunning,
		PID:   vmInfo.PID,
	}, nil
}

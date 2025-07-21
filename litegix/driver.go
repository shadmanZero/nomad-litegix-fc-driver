// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package litegix

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/consul-template/signals"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/drivers/shared/eventer"
	"github.com/hashicorp/nomad/plugins/base"
	"github.com/hashicorp/nomad/plugins/drivers"
	"github.com/hashicorp/nomad/plugins/shared/hclspec"
	"github.com/hashicorp/nomad/plugins/shared/structs"
)

const (
	pluginName = "litegix-fc-driver"
	pluginVersion = "v0.1.0"	
	fingerprintPeriod = 30 * time.Second
	taskHandleVersion = 1
)

var (
	pluginInfo = &base.PluginInfoResponse{
		Type:              base.PluginTypeDriver,
		PluginApiVersions: []string{drivers.ApiVersion010},
		PluginVersion:     pluginVersion,
		Name:              pluginName,
	}
	configSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"vmlinux_path": hclspec.NewAttr("vmlinux_path", "string", true),
		"rootfs_base_path": hclspec.NewAttr("rootfs_base_path", "string", true),
		"containerd_socket": hclspec.NewAttr("containerd_socket", "string", false),
	})


	taskConfigSpec = hclspec.NewObject(map[string]*hclspec.Spec{
		"image" : hclspec.NewAttr("image","string",true),
		"vpu_count" : hclspec.NewAttr("vpu_count","number",true),
		"mem_size" : hclspec.NewAttr("mem_size","number",true),
		"args" : hclspec.NewAttr("args","string",false),
		"command" : hclspec.NewAttr("command","string",false),
		"env" : hclspec.NewAttr("env","list(string)",false),
	})

	capabilities = &drivers.Capabilities{
		SendSignals: true,
		Exec:        false,
	}
)

// Config contains configuration information for the plugin
type Config struct {
	VmlinuxPath     string `codec:"vmlinux_path"`
	RootfsBasePath  string `codec:"rootfs_base_path"`
	ContainerdSocket string `codec:"containerd_socket"`
}

// TaskConfig contains configuration information for a task that runs with
// this plugin
type TaskConfig struct {
	Image    string   `codec:"image"`
	VpuCount int      `codec:"vpu_count"`
	MemSize  int      `codec:"mem_size"`
	Args     string   `codec:"args"`
	Command  string   `codec:"command"`
	Env      []string `codec:"env"`
}

type TaskState struct {
	TaskConfig     *drivers.TaskConfig
	StartedAt      time.Time
	ContainerName string

}

// LitegixDriverPlugin is an example driver plugin. When provisioned in a job,
// the taks will output a greet specified by the user.
type LitegixDriverPlugin struct {
	// eventer is used to handle multiplexing of TaskEvents calls such that an
	// event can be broadcast to all callers
	eventer *eventer.Eventer

	// config is the plugin configuration set by the SetConfig RPC
	config *Config

	// nomadConfig is the client config from Nomad
	nomadConfig *base.ClientDriverConfig

	// tasks is the in memory datastore mapping taskIDs to driver handles
	tasks *taskStore

	// ctx is the context for the driver. It is passed to other subsystems to
	// coordinate shutdown
	ctx context.Context

	// signalShutdown is called when the driver is shutting down and cancels
	// the ctx passed to any subsystems
	signalShutdown context.CancelFunc

	// logger will log to the Nomad agent
	logger hclog.Logger

	// vmManager manages firecracker VMs
	vmManager VMManager
}

// NewPlugin returns a new example driver plugin
func NewPlugin(logger hclog.Logger) drivers.DriverPlugin {
	ctx, cancel := context.WithCancel(context.Background())
	logger = logger.Named(pluginName)

	return &LitegixDriverPlugin{
		eventer:        eventer.NewEventer(ctx, logger),
		config:         &Config{},
		tasks:          newTaskStore(),
		ctx:            ctx,
		signalShutdown: cancel,
		logger:         logger,
		vmManager:      nil, // Will be initialized in SetConfig
	}
}

// PluginInfo returns information describing the plugin.
func (d *LitegixDriverPlugin) PluginInfo() (*base.PluginInfoResponse, error) {
	return pluginInfo, nil
}

// ConfigSchema returns the plugin configuration schema.
func (d *LitegixDriverPlugin) ConfigSchema() (*hclspec.Spec, error) {
	return configSpec, nil
}

// SetConfig is called by the client to pass the configuration for the plugin.
func (d *LitegixDriverPlugin) SetConfig(cfg *base.Config) error {
	var config Config
	if len(cfg.PluginConfig) != 0 {
		if err := base.MsgPackDecode(cfg.PluginConfig, &config); err != nil {
			return err
		}
	}

	// Validate required configuration
	if config.VmlinuxPath == "" {
		return fmt.Errorf("vmlinux_path is required")
	}
	if config.RootfsBasePath == "" {
		return fmt.Errorf("rootfs_base_path is required")
	}

	// Validate that vmlinux exists
	if _, err := os.Stat(config.VmlinuxPath); err != nil {
		return fmt.Errorf("vmlinux_path does not exist: %s", config.VmlinuxPath)
	}

	// Ensure rootfs base directory exists
	if err := os.MkdirAll(config.RootfsBasePath, 0755); err != nil {
		return fmt.Errorf("failed to create rootfs_base_path: %w", err)
	}

	// Save the configuration to the plugin
	d.config = &config

	// Save the Nomad agent configuration
	if cfg.AgentConfig != nil {
		d.nomadConfig = cfg.AgentConfig.Driver
	}

	// Initialize VM manager with the configuration
	d.vmManager = NewVMManager(d.config, d.logger)

	return nil
}

// TaskConfigSchema returns the HCL schema for the configuration of a task.
func (d *LitegixDriverPlugin) TaskConfigSchema() (*hclspec.Spec, error) {
	return taskConfigSpec, nil
}

// Capabilities returns the features supported by the driver.
func (d *LitegixDriverPlugin) Capabilities() (*drivers.Capabilities, error) {
	return capabilities, nil
}

// Fingerprint returns a channel that will be used to send health information
// and other driver specific node attributes.
func (d *LitegixDriverPlugin) Fingerprint(ctx context.Context) (<-chan *drivers.Fingerprint, error) {
	ch := make(chan *drivers.Fingerprint)
	go d.handleFingerprint(ctx, ch)
	return ch, nil
}

// handleFingerprint manages the channel and the flow of fingerprint data.
func (d *LitegixDriverPlugin) handleFingerprint(ctx context.Context, ch chan<- *drivers.Fingerprint) {
	defer close(ch)

	// Nomad expects the initial fingerprint to be sent immediately
	ticker := time.NewTimer(0)
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// after the initial fingerprint we can set the proper fingerprint
			// period
			ticker.Reset(fingerprintPeriod)
			ch <- d.buildFingerprint()
		}
	}
}

// buildFingerprint returns the driver's fingerprint data
func (d *LitegixDriverPlugin) buildFingerprint() *drivers.Fingerprint {
	fp := &drivers.Fingerprint{
		Attributes:        map[string]*structs.Attribute{},
		Health:            drivers.HealthStateHealthy,
		HealthDescription: drivers.DriverHealthy,
	}

	// TODO: implement fingerprinting logic to populate health and driver
	// attributes.
	//
	// Fingerprinting is used by the plugin to relay two important information
	// to Nomad: health state and node attributes.
	//
	// If the plugin reports to be unhealthy, or doesn't send any fingerprint
	// data in the expected interval of time, Nomad will restart it.
	//
	// Node attributes can be used to report any relevant information about
	// the node in which the plugin is running (specific library availability,
	// installed versions of a software etc.). These attributes can then be
	// used by an operator to set job constrains.

	return fp
}

// StartTask returns a task handle and a driver network if necessary.
func (d *LitegixDriverPlugin) StartTask(cfg *drivers.TaskConfig) (*drivers.TaskHandle, *drivers.DriverNetwork, error) {
	if _, ok := d.tasks.Get(cfg.ID); ok {
		return nil, nil, fmt.Errorf("task with ID %q already started", cfg.ID)
	}

	var driverConfig TaskConfig
	if err := cfg.DecodeDriverConfig(&driverConfig); err != nil {
		return nil, nil, fmt.Errorf("failed to decode driver config: %v", err)
	}

	d.logger.Info("starting task", "driver_cfg", hclog.Fmt("%+v", driverConfig))

	// Create and start the VM using the VM manager
	ctx := context.Background()
	vmInfo, err := d.vmManager.CreateAndStartVM(ctx, &driverConfig, cfg.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create and start VM: %w", err)
	}

	// Create task handle
	handle := drivers.NewTaskHandle(taskHandleVersion)
	handle.Config = cfg

	// Create our internal task handle
	h := &taskHandle{
		taskConfig: cfg,
		logger:     d.logger.With("task_id", cfg.ID),
		startedAt:  time.Now(),
		procState:  drivers.TaskStateRunning,
		vmInfo:     vmInfo,
		vmManager:  d.vmManager,
	}

	// Save driver state
	driverState := TaskState{
		TaskConfig:    cfg,
		ContainerName: fmt.Sprintf("%s-%s", cfg.Name, cfg.AllocID),
		StartedAt:     h.startedAt,
	}

	if err := handle.SetDriverState(&driverState); err != nil {
		// Clean up VM on error
		d.vmManager.DestroyVM(ctx, vmInfo)
		return nil, nil, fmt.Errorf("failed to set driver state: %v", err)
	}

	d.tasks.Set(cfg.ID, h)
	go h.run()
	return handle, nil, nil
}

// RecoverTask recreates the in-memory state of a task from a TaskHandle.
func (d *LitegixDriverPlugin) RecoverTask(handle *drivers.TaskHandle) error {
	if handle == nil {
		return errors.New("error: handle cannot be nil")
	}

	if _, ok := d.tasks.Get(handle.Config.ID); ok {
		return nil
	}

	var taskState TaskState
	if err := handle.GetDriverState(&taskState); err != nil {
		return fmt.Errorf("failed to decode task state from handle: %v", err)
	}

	var driverConfig TaskConfig
	if err := taskState.TaskConfig.DecodeDriverConfig(&driverConfig); err != nil {
		return fmt.Errorf("failed to decode driver config: %v", err)
	}

	d.logger.Info("recovering task", "task_id", handle.Config.ID)

	// Note: For firecracker VMs, full recovery is complex since VMs are ephemeral
	// In a production implementation, you might want to store VM state information
	// in the TaskState and attempt to reconnect to running VMs
	h := &taskHandle{
		taskConfig: taskState.TaskConfig,
		logger:     d.logger.With("task_id", handle.Config.ID),
		startedAt:  taskState.StartedAt,
		procState:  drivers.TaskStateUnknown, // Mark as unknown since VM state is unclear
		vmManager:  d.vmManager,
		// vmInfo: nil, // VM info cannot be recovered without persistent state
	}

	d.tasks.Set(taskState.TaskConfig.ID, h)
	go h.run()
	return nil
}

// WaitTask returns a channel used to notify Nomad when a task exits.
func (d *LitegixDriverPlugin) WaitTask(ctx context.Context, taskID string) (<-chan *drivers.ExitResult, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	ch := make(chan *drivers.ExitResult)
	go d.handleWait(ctx, handle, ch)
	return ch, nil
}

func (d *LitegixDriverPlugin) handleWait(ctx context.Context, handle *taskHandle, ch chan *drivers.ExitResult) {
	defer close(ch)
	
	// Wait for the task handle's run method to complete
	// The run method will update the handle's exit result when the VM stops
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			handle.stateLock.RLock()
			if handle.procState == drivers.TaskStateExited || 
			   handle.procState == drivers.TaskStateUnknown {
				result := handle.exitResult
				if result == nil {
					result = &drivers.ExitResult{}
				}
				handle.stateLock.RUnlock()
				
				// Send the result and exit
				select {
				case <-ctx.Done():
					return
				case <-d.ctx.Done():
					return
				case ch <- result:
					return
				}
			}
			handle.stateLock.RUnlock()
		}
	}
}

// StopTask stops a running task with the given signal and within the timeout window.
func (d *LitegixDriverPlugin) StopTask(taskID string, timeout time.Duration, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	d.logger.Info("stopping task", "task_id", taskID, "timeout", timeout, "signal", signal)

	// Use the VM manager to stop the VM
	ctx := context.Background()
	if err := d.vmManager.StopVM(ctx, handle.vmInfo, timeout); err != nil {
		d.logger.Error("failed to stop VM", "task_id", taskID, "error", err)
		return fmt.Errorf("failed to stop VM: %w", err)
	}

	return nil
}

// DestroyTask cleans up and removes a task that has terminated.
func (d *LitegixDriverPlugin) DestroyTask(taskID string, force bool) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	if handle.IsRunning() && !force {
		return errors.New("cannot destroy running task")
	}

	d.logger.Info("destroying task", "task_id", taskID, "force", force)

	// Use the VM manager to destroy the VM
	ctx := context.Background()
	if err := d.vmManager.DestroyVM(ctx, handle.vmInfo); err != nil {
		d.logger.Error("failed to destroy VM", "task_id", taskID, "error", err)
		// Continue with cleanup even if VM destruction fails
	}

	d.tasks.Delete(taskID)
	return nil
}

// InspectTask returns detailed status information for the referenced taskID.
func (d *LitegixDriverPlugin) InspectTask(taskID string) (*drivers.TaskStatus, error) {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	return handle.TaskStatus(), nil
}

// TaskStats returns a channel which the driver should send stats to at the given interval.
func (d *LitegixDriverPlugin) TaskStats(ctx context.Context, taskID string, interval time.Duration) (<-chan *drivers.TaskResourceUsage, error) {
	_, ok := d.tasks.Get(taskID)
	if !ok {
		return nil, drivers.ErrTaskNotFound
	}

	// For now, return basic stats - in a production implementation you might
	// want to collect actual VM resource usage statistics
	ch := make(chan *drivers.TaskResourceUsage)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Send basic stats - in production you'd collect real VM stats
				stats := &drivers.TaskResourceUsage{
					ResourceUsage: &drivers.ResourceUsage{
						MemoryStats: &drivers.MemoryStats{},
						CpuStats:    &drivers.CpuStats{},
					},
					Timestamp: time.Now().UTC().UnixNano(),
					Pids:      map[string]*drivers.ResourceUsage{},
				}

				select {
				case <-ctx.Done():
					return
				case ch <- stats:
				}
			}
		}
	}()

	return ch, nil
}

// TaskEvents returns a channel that the plugin can use to emit task related events.
func (d *LitegixDriverPlugin) TaskEvents(ctx context.Context) (<-chan *drivers.TaskEvent, error) {
	return d.eventer.TaskEvents(ctx)
}

// SignalTask forwards a signal to a task.
// This is an optional capability.
func (d *LitegixDriverPlugin) SignalTask(taskID string, signal string) error {
	handle, ok := d.tasks.Get(taskID)
	if !ok {
		return drivers.ErrTaskNotFound
	}

	d.logger.Info("sending signal to task", "task_id", taskID, "signal", signal)

	// For firecracker VMs, we can send signals to the VM process
	if handle.vmInfo == nil || handle.vmInfo.Machine == nil {
		return fmt.Errorf("VM info not available for task %s", taskID)
	}

	pid, err := handle.vmInfo.Machine.PID()
	if err != nil {
		return fmt.Errorf("failed to get VM PID: %w", err)
	}

	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find VM process: %w", err)
	}

	// Convert signal string to os.Signal
	sig := os.Interrupt
	if s, ok := signals.SignalLookup[signal]; ok {
		sig = s
	} else {
		d.logger.Warn("unknown signal to send to task, using SIGINT instead", "signal", signal, "task_id", taskID)
	}

	return process.Signal(sig)
}

// ExecTask returns the result of executing the given command inside a task.
// This is an optional capability.
func (d *LitegixDriverPlugin) ExecTask(taskID string, cmd []string, timeout time.Duration) (*drivers.ExecTaskResult, error) {
	// Firecracker VMs don't support exec - would need additional agent inside VM
	return nil, errors.New("This driver does not support exec")
}

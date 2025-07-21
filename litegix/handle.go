// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package litegix

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/plugins/drivers"
)

// taskHandle should store all relevant runtime information
// such as process ID if this is a local task or other meta
// data if this driver deals with external APIs
type taskHandle struct {
	// stateLock syncs access to all fields below
	stateLock    sync.RWMutex
	taskConfig   *drivers.TaskConfig
	logger       hclog.Logger
	startedAt    time.Time
	completedAt  time.Time
	exitResult   *drivers.ExitResult
	procState    drivers.TaskState
	vmInfo       *VMInfo
	vmManager    VMManager
}

func (h *taskHandle) TaskStatus() *drivers.TaskStatus {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()

	var pid string
	var vmID string
	if h.vmInfo != nil {
		pid = strconv.Itoa(int(h.vmInfo.PID))
		vmID = h.vmInfo.VMID
	}

	return &drivers.TaskStatus{
		ID:          h.taskConfig.ID,
		Name:        h.taskConfig.Name,
		State:       h.procState,
		StartedAt:   h.startedAt,
		CompletedAt: h.completedAt,
		ExitResult:  h.exitResult,
		DriverAttributes: map[string]string{
			"pid":    pid,
			"vm_id":  vmID,
		},
	}
}

func (h *taskHandle) IsRunning() bool {
	h.stateLock.RLock()
	defer h.stateLock.RUnlock()
	return h.procState == drivers.TaskStateRunning
}

func (h *taskHandle) run() {
	h.stateLock.Lock()
	if h.exitResult == nil {
		h.exitResult = &drivers.ExitResult{}
	}
	h.stateLock.Unlock()

	// Monitor the VM status
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			status, err := h.vmManager.GetVMStatus(ctx, h.vmInfo)
			cancel()

			h.stateLock.Lock()
			if err != nil {
				h.logger.Error("failed to get VM status", "error", err)
				h.exitResult.Err = err
				h.procState = drivers.TaskStateUnknown
				h.completedAt = time.Now()
				h.stateLock.Unlock()
				return
			}

			switch status.State {
			case VMStateRunning:
				h.procState = drivers.TaskStateRunning
			case VMStateStopped:
				h.procState = drivers.TaskStateExited
				if status.ExitCode != nil {
					h.exitResult.ExitCode = int(*status.ExitCode)
				}
				if status.ExitedAt != nil {
					h.completedAt = *status.ExitedAt
				} else {
					h.completedAt = time.Now()
				}
				h.stateLock.Unlock()
				return
			case VMStateUnknown:
				h.procState = drivers.TaskStateUnknown
				h.completedAt = time.Now()
				h.stateLock.Unlock()
				return
			}
			h.stateLock.Unlock()
		}
	}
}

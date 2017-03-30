// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/lxc/lxd/shared/api"
)

// The various status values used for LXD.
const (
	StatusStarting = "Starting"
	StatusStarted  = "Started"
	StatusRunning  = "Running"
	StatusFreezing = "Freezing"
	StatusFrozen   = "Frozen"
	StatusThawed   = "Thawed"
	StatusStopping = "Stopping"
	StatusStopped  = "Stopped"

	StatusOperationCreated = "Operation created"
	StatusPending          = "Pending"
	StatusAborting         = "Aborting"
	StatusCancelling       = "Canceling"
	StatusCancelled        = "Canceled"
	StatusSuccess          = "Success"
	StatusFailure          = "Failure"
)

var allStatuses = map[string]api.StatusCode{
	StatusStarting:         api.Starting,
	StatusStarted:          api.Started,
	StatusRunning:          api.Running,
	StatusFreezing:         api.Freezing,
	StatusFrozen:           api.Frozen,
	StatusThawed:           api.Thawed,
	StatusStopping:         api.Stopping,
	StatusStopped:          api.Stopped,
	StatusOperationCreated: api.OperationCreated,
	StatusPending:          api.Pending,
	StatusAborting:         api.Aborting,
	StatusCancelling:       api.Cancelling,
	StatusCancelled:        api.Cancelled,
	StatusSuccess:          api.Success,
	StatusFailure:          api.Failure,
}

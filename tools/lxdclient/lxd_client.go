// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build go1.3

package lxdclient

import (
	"github.com/lxc/lxd/shared"
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

var allStatuses = map[string]shared.StatusCode{
	StatusStarting:         shared.Starting,
	StatusStarted:          shared.Started,
	StatusRunning:          shared.Running,
	StatusFreezing:         shared.Freezing,
	StatusFrozen:           shared.Frozen,
	StatusThawed:           shared.Thawed,
	StatusStopping:         shared.Stopping,
	StatusStopped:          shared.Stopped,
	StatusOperationCreated: shared.OperationCreated,
	StatusPending:          shared.Pending,
	StatusAborting:         shared.Aborting,
	StatusCancelling:       shared.Cancelling,
	StatusCancelled:        shared.Cancelled,
	StatusSuccess:          shared.Success,
	StatusFailure:          shared.Failure,
}

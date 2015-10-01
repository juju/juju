// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd_client

import (
	"github.com/juju/loggo"
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

	StatusOK         = "OK"
	StatusPending    = "Pending"
	StatusAborting   = "Aborting"
	StatusCancelling = "Canceling"
	StatusCancelled  = "Canceled"
	StatusSuccess    = "Success"
	StatusFailure    = "Failure"
)

var allStatuses = map[string]shared.State{
	StatusStarting: shared.STARTING,
	StatusRunning:  shared.RUNNING,
	StatusThawed:   shared.THAWED,

	// TODO(ericsnow) Use the newer status codes:
	//StatusStarting:   shared.Starting,
	//StatusStarted:    shared.Started,
	//StatusRunning:    shared.Running,
	//StatusFreezing:   shared.Freezing,
	//StatusFrozen:     shared.Frozen,
	//StatusThawed:     shared.Thawed,
	//StatusStopping:   shared.Stopping,
	//StatusStopped:    shared.Stopped,
	//StatusOK:         shared.OK,
	//StatusPending:    shared.Pending,
	//StatusAborting:   shared.Aborting,
	//StatusCancelling: shared.Cancelling,
	//StatusCancelled:  shared.Cancelled,
	//StatusSuccess:    shared.Success,
	//StatusFailure:    shared.Failure,
}

var (
	logger = loggo.GetLogger("juju.provider.lxd.lxd_client")
)

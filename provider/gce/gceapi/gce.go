// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"github.com/juju/loggo"
)

const (
	StatusDone         = "DONE"
	StatusDown         = "DOWN"
	StatusPending      = "PENDING"
	StatusProvisioning = "PROVISIONING"
	StatusRunning      = "RUNNING"
	StatusStaging      = "STAGING"
	StatusStopped      = "STOPPED"
	StatusStopping     = "STOPPING"
	StatusTerminated   = "TERMINATED"
	StatusUp           = "UP"
)

var (
	logger = loggo.GetLogger("juju.provider.gce.gceapi")
)

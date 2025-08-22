// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"github.com/juju/loggo"
)

var logger = loggo.GetLogger("juju.provider.gce.gceapi")

// The various status values used by GCE.
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

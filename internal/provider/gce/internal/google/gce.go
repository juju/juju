// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	internallogger "github.com/juju/juju/internal/logger"
)

var (
	logger = internallogger.GetLogger("juju.provider.gce.gceapi")
)

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

// HostMaintenanceTerminate is a host maintenance policy that terminates instances
// instead of performing live migration (e.g., for GPU instances).
const (
	HostMaintenanceTerminate = "TERMINATE"
)

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"path"

	"code.google.com/p/google-api-go-client/compute/v1"
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

func zoneName(value interface{}) string {
	// We trust that path.Base will always give the right answer
	// when used.
	switch typed := value.(type) {
	case *compute.Instance:
		return path.Base(typed.Zone)
	case *compute.Operation:
		return path.Base(typed.Zone)
	default:
		// TODO(ericsnow) Fail?
		return ""
	}
}

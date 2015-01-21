// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

// These are the possible service statuses.
const (
	StatusDisabled = "disabled"
	StatusStopped  = "stopped"
	StatusStarting = "starting"
	StatusRunning  = "running"
	StatusStopping = "stopping"
)

// ServiceInfo holds information about an init service, as gathered at
// some moment in time.
type ServiceInfo struct {
	// Name is the name of the service.
	Name string

	// Description is the human-readable description of the service.
	Description string

	// Status describes the status of the service.
	Status string
}

// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

// StatusInfo is a record of the status information for an application or a unit's workload.
type StatusInfo struct {
	Tag    string
	Status string
	Info   string
	Data   map[string]interface{}
}

// ApplicationStatusInfo holds StatusInfo for an application and all its units.
type ApplicationStatusInfo struct {
	Application StatusInfo
	Units       []StatusInfo
}

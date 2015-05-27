// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

// ProcessDetails holds information about an existing process as provided by
// a workload process plugin.
type ProcessDetails struct {
	// UniqueID is provided by the plugin as a guaranteed way
	// to identify the process to the plugin.
	UniqueID string

	// Status is the status of the process as reported by the plugin.
	Status string
}

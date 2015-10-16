// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The workload package (and subpackages) contain the implementation of
// the charm workload feature component. The various pieces are
// connected to the Juju machinery in component/all/workload.go.
package workload

// ComponentName is the name of the Juju component for workload management.
const ComponentName = "workloads"

// Result is a struct that ties an error to a workload ID.
type Result struct {
	// FullID is the ID of the workload that this result applies to.
	FullID string
	// Err is the error associated with this result (if any).
	Err error
}

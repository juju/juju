// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The workload package (and subpackages) contain the implementation of
// the charm workload feature component. The various pieces are
// connected to the Juju machinery in component/all/workload.go.
package workload

// ComponentName is the name of the Juju component for workload management.
const ComponentName = "workloads"

// WorkloadError is a struct that ties an error to a workload ID.
type WorkloadError struct {
	ID  string
	Err error
}

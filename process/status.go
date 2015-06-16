// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"fmt"
)

// Status values specific to workload processes.
const (
	StatusPending Status = iota
	StatusActive
	StatusFailed
	StatusStopped
)

// knownStatuses defines the statuses supported by Juju and their string.
var knownStatuses = map[Status]string{
	StatusPending: "pending",
	StatusActive:  "active",
	StatusFailed:  "failed",
	StatusStopped: "stopped",
}

// Status represents the status of a workload process.
type Status int

// IsUnnown returns true if the status is not known to Juju.
func (s Status) IsUnknown() bool {
	_, ok := knownStatuses[s]
	return !ok
}

// String implements fmt.Stringer.
func (s Status) String() string {
	if statusStr, ok := knownStatuses[s]; ok {
		return statusStr
	}
	return "unknown"
}

// String implements fmt.Gostringer.
func (s Status) GoString() string {
	return fmt.Sprintf("<%T %q>", s, s)
}

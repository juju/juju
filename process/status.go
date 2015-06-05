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

// knownStatuses defines the statuses supported by Juju.
var knownStatuses = []Status{
	StatusPending,
	StatusActive,
	StatusFailed,
	StatusStopped,
}

// Status represents the status of a worload process.
type Status int

// IsUnnown returns true if the status is not known to Juju.
func (s Status) IsUnknown() bool {
	return s.String() == "unknown"
}

// String implements fmt.Stringer.
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusActive:
		return "active"
	case StatusFailed:
		return "failed"
	case StatusStopped:
		return "stopped"
	}
	return "unknown"
}

// String implements fmt.Gostringer.
func (s Status) GoString() string {
	return fmt.Sprintf("<%T %q>", s, s)
}

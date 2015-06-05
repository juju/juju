// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

// Status values specific to workload processes.
const (
	StatusPending Status = 1 << iota
	StatusActive
	StatusFailed
	StatusStopped
)

// KnownStatuses defines the statuses supported by Juju.
var KnownStatuses = []Status{
	StatusPending,
	StatusActive,
	StatusFailed,
	StatusStopped,
}

// Status represents the status of a worload process.
type Status int

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

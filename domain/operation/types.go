// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"time"

	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/uuid"
)

// Action represents a domain action.
type Action struct {
	// UUID is the action unique identifier.
	UUID uuid.UUID
	// Receiver is the action receiver (unit / machine).
	Receiver string
}

// QueryArgs represents the parameters used for querying operations.
type QueryArgs struct {
	Target
	ActionNames []string
	Status      []string

	// These attributes are used to support client side
	// batching of results.
	Limit  *int
	Offset *int
}

// QueryResult represents the result of a query operation.
type QueryResult struct {
	Operations []OperationInfo
	Truncated  bool
}

// OperationInfo represents the information about an operation.
type OperationInfo struct {
	OperationID string
	Summary     string
	Fail        string
	Enqueued    time.Time
	Started     time.Time
	Completed   time.Time
	Status      string
	Machines    []MachineTaskResult
	Units       []UnitTaskResult
	Truncated   bool
	Error       error
}

// RunArgs represents the parameters used for running operations.
type RunArgs struct {
	Target
	TaskArgs
}

// TaskArgs represents the parameters used for running tasks.
type TaskArgs struct {
	ActionName     string
	Parameters     map[string]interface{}
	IsParallel     bool
	ExecutionGroup string
}

// RunResult represents the result of a run operation.
type RunResult struct {
	OperationID string
	Machines    []MachineTaskResult
	Units       []UnitTaskResult
}

// MachineTaskResult represents the result of a machine task.
type MachineTaskResult struct {
	TaskInfo
	ReceiverName machine.Name
}

// UnitTaskResult represents the result of a unit task.
type UnitTaskResult struct {
	TaskInfo
	ReceiverName unit.Name
	IsLeader     bool
}

// TaskInfo represents the information about a task.
type TaskInfo struct {
	TaskArgs
	ID        string
	Enqueued  time.Time
	Started   time.Time
	Completed time.Time
	Status    string
	Message   string
	Log       []TaskLog
	Output    map[string]interface{}
	Error     error
}

// TaskLog represents a log message for a task.
type TaskLog struct {
	Timestamp time.Time
	Message   string
}

// Target represents various targets for operations.
type Target struct {
	Applications []string
	Machines     []machine.Name
	Units        []unit.Name
	LeaderUnit   []string
}

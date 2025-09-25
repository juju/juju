// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

import (
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/internal/errors"
)

// CompletedTaskResult holds the task ID and output used when recording
// the result of an task.
type CompletedTaskResult struct {
	TaskID  string
	Status  string
	Results map[string]interface{}
	Message string
}

func (c CompletedTaskResult) Validate() error {
	var errList []error
	if c.TaskID == "" {
		errList = append(errList, errors.Errorf("TaskID is empty").Add(coreerrors.NotValid))
	}
	status := corestatus.Status(c.Status)
	if !status.IsInActiveTaskStatus() {
		errList = append(errList, errors.Errorf("Status is not valid for completed tasks").Add(coreerrors.NotValid))
	}
	return errors.Join(errList...)
}

// QueryArgs represents the parameters used for querying operations.
type QueryArgs struct {
	// ActionNames defines which specific action names we want to retrieve.
	// If empty, all operations will be retrieved among exec or actions operations
	ActionNames []string

	// Receivers defines a filter on which receiver(s) we want to retrieve operations.
	// if empty, operations from all receivers will be retrieved.
	Receivers Receivers

	// Status defines which specific status we want to retrieve.
	// If empty, operations with any status will be retrieved.
	Status []corestatus.Status

	// These attributes are used to support client side
	// batching of results.
	Limit  *int
	Offset *int
}

// QueryResult contains the result of a query request for operations.
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
	Status      corestatus.Status
	Machines    []MachineTaskResult
	Units       []UnitTaskResult

	// Truncated indicates that there are more results to be fetched, but the whole
	// result set has been truncated to either the limit passed as a query
	// parameter or the default limit on the server side.
	Truncated bool
	Error     error
}

// ExecArgs represents the parameters used for running exec commands.
type ExecArgs struct {
	Command        string
	Timeout        time.Duration
	Parallel       bool
	ExecutionGroup string
}

// TaskArgs represents the parameters used for running tasks.
type TaskArgs struct {
	ActionName     string
	ExecutionGroup string
	IsParallel     bool
	Parameters     map[string]interface{}
}

// RunResult represents the result of a run operation.
type RunResult struct {
	OperationID string

	Machines []MachineTaskResult
	Units    []UnitTaskResult
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

// Task represents the task as returned to the apiserver (which will convert
// the string to an actionReceiverTag and back to string).
type Task struct {
	TaskInfo

	Receiver string
}

// TaskInfo represents the information about a task.
type TaskInfo struct {
	ID string

	ActionName     string
	ExecutionGroup *string
	IsParallel     bool
	Parameters     map[string]interface{}

	Completed time.Time
	Enqueued  time.Time
	Error     error
	Log       []TaskLog
	Message   string
	Output    map[string]any
	Started   time.Time
	Status    corestatus.Status
}

// TaskLog represents a log message for a task.
type TaskLog struct {
	Timestamp time.Time
	Message   string
}

// Receivers represents various receivers for operations.
type Receivers struct {
	Applications []string
	Machines     []machine.Name
	Units        []unit.Name
	LeaderUnit   []string
}

// ActionReceiver allows running an action on a specific unit or a leader unit of an application
// only one of both fields should be set.
type ActionReceiver struct {
	Unit       unit.Name
	LeaderUnit string
}

// Validate checks that the ActionReceiver is correctly configured, i.e. only
// one of Unit or LeaderUnit is set, and at least one is set.
func (a *ActionReceiver) Validate() error {
	if a.Unit != "" && a.LeaderUnit != "" {
		return errors.Errorf("only one of Unit or LeaderUnit should be set")
	}
	if a.Unit == "" && a.LeaderUnit == "" {
		return errors.Errorf("one of Unit or LeaderUnit must be set")
	}
	return nil
}

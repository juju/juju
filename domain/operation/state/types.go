// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"
)

// uuids represents a slice of UUIDs.
type uuids []string

// taskResult represents the result of joining operation with its tasks and
// receivers.
type taskResult struct {
	OperationUUID  string         `db:"operation_uuid"`
	Receiver       string         `db:"receiver"`
	Name           sql.NullString `db:"name"`
	Summary        sql.NullString `db:"summary"`
	Parallel       bool           `db:"parallel"`
	ExecutionGroup sql.NullString `db:"execution_group"`
	EnqueuedAt     time.Time      `db:"enqueued_at"`
	StartedAt      sql.NullTime   `db:"started_at"`
	CompletedAt    sql.NullTime   `db:"completed_at"`
	Status         string         `db:"status"`
	StatusMessage  sql.NullString `db:"status_message"`
	StatusValue    sql.NullString `db:"status_value"`
	OutputPath     sql.NullString `db:"path"`
}

// uuid represents a simple wrapper for operation UUID queries.
type uuid struct {
	UUID string `db:"uuid"`
}

// taskParameter represents a parameter key-value pair for a task.
type taskParameter struct {
	OperationUUID string `db:"operation_uuid"`
	Key           string `db:"key"`
	Value         string `db:"value"`
}

// taskLogEntry represents a log entry from operation_task_log.
type taskLogEntry struct {
	TaskUUID  string    `db:"task_uuid"`
	Content   string    `db:"content"`
	CreatedAt time.Time `db:"created_at"`
}

// taskIdent represents a task ID parameter for queries.
type taskIdent struct {
	ID string `db:"task_id"`
}

// taskStatus represents a task status for queries on the
// operation_task_status table.
type taskStatus struct {
	TaskUUID  string    `db:"task_uuid"`
	Status    string    `db:"status"`
	Message   string    `db:"message"`
	UpdatedAt time.Time `db:"updated_at"`
}

type nameArg struct {
	Name string `db:"name"`
}

// path represents a path parameter for queries on the
// object_store_metadata_path table.
type path struct {
	Path string `db:"path"`
}

// taskTime maps a task ID and time together
type taskTime struct {
	TaskID string    `db:"task_id"`
	Time   time.Time `db:"time"`
}

type pagination struct {
	Cursor time.Time `db:"cursor"`
}

// taskUUIDTime maps a task UUID and time together
type taskUUIDTime struct {
	TaskUUID string    `db:"task_uuid"`
	Time     time.Time `db:"time"`
}

// outputStore contains the data to interact with the
// operation_task_output table.
type outputStore struct {
	TaskUUID  string `db:"task_uuid"`
	StoreUUID string `db:"store_uuid"`
}

type insertOperation struct {
	UUID           string    `db:"uuid"`
	OperationID    string    `db:"operation_id"`
	Summary        string    `db:"summary"`
	EnqueuedAt     time.Time `db:"enqueued_at"`
	Parallel       bool      `db:"parallel"`
	ExecutionGroup string    `db:"execution_group"`
}

type insertOperationAction struct {
	OperationUUID  string `db:"operation_uuid"`
	CharmUUID      string `db:"charm_uuid"`
	CharmActionKey string `db:"charm_action_key"`
}

type insertOperationTask struct {
	UUID          string    `db:"uuid"`
	OperationUUID string    `db:"operation_uuid"`
	TaskID        string    `db:"task_id"`
	EnqueuedAt    time.Time `db:"enqueued_at"`
}

type insertTaskStatus struct {
	TaskUUID  string    `db:"task_uuid"`
	Status    string    `db:"status"`
	UpdatedAt time.Time `db:"updated_at"`
}

type insertUnitTask struct {
	TaskUUID string `db:"task_uuid"`
	UnitUUID string `db:"unit_uuid"`
}

type insertMachineTask struct {
	TaskUUID    string `db:"task_uuid"`
	MachineUUID string `db:"machine_uuid"`
}

type charmUUIDResult struct {
	CharmUUID string `db:"charm_uuid"`
}

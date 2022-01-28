// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package actionstate

import "time"

// ActionStatus represents the possible end states for an action.
type ActionStatus string

const (
	// ActionError signifies that the action did get run due to an error.
	ActionError ActionStatus = "error"

	// ActionFailed signifies that the action did not complete successfully.
	ActionFailed ActionStatus = "failed"

	// ActionCompleted indicates that the action ran to completion as intended.
	ActionCompleted ActionStatus = "completed"

	// ActionCancelled means that the Action was cancelled before being run.
	ActionCancelled ActionStatus = "cancelled"

	// ActionPending is the default status when an Action is first queued.
	ActionPending ActionStatus = "pending"

	// ActionRunning indicates that the Action is currently running.
	ActionRunning ActionStatus = "running"

	// ActionAborting indicates that the Action is running but should be
	// aborted.
	ActionAborting ActionStatus = "aborting"

	// ActionAborted indicates the Action was aborted.
	ActionAborted ActionStatus = "aborted"
)

// ActionMessage represents a progress message logged by an action.
type ActionMessage struct {
	MessageValue   string
	TimestampValue time.Time
}

// Timestamp returns the message timestamp.
func (m ActionMessage) Timestamp() time.Time {
	return m.TimestampValue
}

// Message returns the message string.
func (m ActionMessage) Message() string {
	return m.MessageValue
}

type Action struct {
	// Receiver is the Name of the Unit or any other ActionReceiver for
	// which this Action is queued.
	Receiver string

	// Name identifies the action that should be run; it should
	// match an action defined by the unit's charm.
	Name string

	// Parameters holds the action's parameters, if any; it should validate
	// against the schema defined by the named action in the unit's charm.
	Parameters map[string]interface{}

	// Enqueued is the time the action was added.
	Enqueued time.Time

	// Started reflects the time the action began running.
	Started time.Time

	// Completed reflects the time that the action was finished.
	Completed time.Time

	// Operation is the parent operation of the action.
	Operation string

	// Status represents the end state of the Action; ActionFailed for an
	// action that was removed prematurely, or that failed, and
	// ActionCompleted for an action that successfully completed.
	Status ActionStatus

	// Message captures any error returned by the action.
	Message string

	// Results are the structured results from the action.
	Results map[string]interface{}

	// Logs holds the progress messages logged by the action.
	Logs []ActionMessage
}

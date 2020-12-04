// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import "time"

const (
	// ActionCancelled is the status for an Action that has been
	// cancelled prior to execution.
	ActionCancelled string = "cancelled"

	// ActionCompleted is the status of an Action that has completed
	// successfully.
	ActionCompleted string = "completed"

	// ActionFailed is the status of an Action that has completed with
	// an error.
	ActionFailed string = "failed"

	// ActionPending is the status of an Action that has been queued up but
	// not executed yet.
	ActionPending string = "pending"

	// ActionRunning is the status of an Action that has been started but
	// not completed yet.
	ActionRunning string = "running"

	// ActionAborting is the status of an Action that is running but is to be
	// terminated. Identical to ActionRunning.
	ActionAborting string = "aborting"

	// ActionAborted is the status of an Action that was aborted.
	// Identical to ActionFailed and can have an error.
	ActionAborted string = "aborted"
)

// Actions is a slice of Action for bulk requests.
type Actions struct {
	Actions []Action `json:"actions,omitempty"`
}

// Action describes an Action that will be or has been queued up.
type Action struct {
	Tag        string                 `json:"tag"`
	Receiver   string                 `json:"receiver"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// EnqueuedActions represents the result of enqueuing actions to run.
type EnqueuedActions struct {
	OperationTag string         `json:"operation"`
	Actions      []ActionResult `json:"actions,omitempty"`
}

// ActionResults is a slice of ActionResult for bulk requests.
type ActionResults struct {
	Results []ActionResult `json:"results,omitempty"`
}

// ActionMessage represents a logged message on an action.
type ActionMessage struct {
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
}

// ActionResult describes an Action that will be or has been completed.
type ActionResult struct {
	Action    *Action                `json:"action,omitempty"`
	Enqueued  time.Time              `json:"enqueued,omitempty"`
	Started   time.Time              `json:"started,omitempty"`
	Completed time.Time              `json:"completed,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Log       []ActionMessage        `json:"log,omitempty"`
	Output    map[string]interface{} `json:"output,omitempty"`
	Error     *Error                 `json:"error,omitempty"`
}

// ActionsByReceivers wrap a slice of Actions for API calls.
type ActionsByReceivers struct {
	Actions []ActionsByReceiver `json:"actions,omitempty"`
}

// ActionsByReceiver is a bulk API call wrapper containing Actions,
// either as input parameters or as results.
type ActionsByReceiver struct {
	Receiver string         `json:"receiver,omitempty"`
	Actions  []ActionResult `json:"actions,omitempty"`
	Error    *Error         `json:"error,omitempty"`
}

// ActionsQueryResults holds a slice of responses from the Actions
// query.
type ActionsQueryResults struct {
	Results []ActionsQueryResult `json:"results,omitempty"`
}

// ActionsQueryResult holds the name and parameters of an query result.
type ActionsQueryResult struct {
	Receiver string       `json:"receiver,omitempty"`
	Action   ActionResult `json:"action,omitempty"`
	Error    *Error       `json:"error,omitempty"`
}

// OperationQueryArgs holds args for listing operations.
type OperationQueryArgs struct {
	Applications []string `json:"applications,omitempty"`
	Units        []string `json:"units,omitempty"`
	Machines     []string `json:"machines,omitempty"`
	ActionNames  []string `json:"actions,omitempty"`
	Status       []string `json:"status,omitempty"`

	// These attributes are used to support client side
	// batching of results.
	Offset *int `json:"offset,omitempty"`
	Limit  *int `json:"limit,omitempty"`
}

// OperationResults is a slice of OperationResult for bulk requests.
type OperationResults struct {
	Results   []OperationResult `json:"results,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
}

// OperationResult describes an Operation that will be or has been completed.
type OperationResult struct {
	OperationTag string         `json:"operation"`
	Summary      string         `json:"summary"`
	Enqueued     time.Time      `json:"enqueued,omitempty"`
	Started      time.Time      `json:"started,omitempty"`
	Completed    time.Time      `json:"completed,omitempty"`
	Status       string         `json:"status,omitempty"`
	Actions      []ActionResult `json:"actions,omitempty"`
	Error        *Error         `json:"error,omitempty"`
}

// ActionExecutionResults holds a slice of ActionExecutionResult for a
// bulk action API call
type ActionExecutionResults struct {
	Results []ActionExecutionResult `json:"results,omitempty"`
}

// ActionExecutionResult holds the action tag and output used when
// recording the result of an action.
type ActionExecutionResult struct {
	ActionTag string                 `json:"action-tag"`
	Status    string                 `json:"status"`
	Results   map[string]interface{} `json:"results,omitempty"`
	Message   string                 `json:"message,omitempty"`
}

// ApplicationsCharmActionsResults holds a slice of ApplicationCharmActionsResult for
// a bulk result of charm Actions for Applications.
type ApplicationsCharmActionsResults struct {
	Results []ApplicationCharmActionsResult `json:"results,omitempty"`
}

// ApplicationCharmActionsResult holds application name and charm.Actions for the application.
// If an error such as a missing charm or malformed application name occurs, it
// is encapsulated in this type.
type ApplicationCharmActionsResult struct {
	ApplicationTag string                `json:"application-tag,omitempty"`
	Actions        map[string]ActionSpec `json:"actions,omitempty"`
	Error          *Error                `json:"error,omitempty"`
}

// ActionSpec is a definition of the parameters and traits of an Action.
// The Params map is expected to conform to JSON-Schema Draft 4 as defined at
// http://json-schema.org/draft-04/schema# (see http://json-schema.org/latest/json-schema-core.html)
type ActionSpec struct {
	Description string                 `json:"description"`
	Params      map[string]interface{} `json:"params"`
}

type ActionPruneArgs struct {
	MaxHistoryTime time.Duration `json:"max-history-time"`
	MaxHistoryMB   int           `json:"max-history-mb"`
}

// ActionMessageParams holds the arguments for
// logging progress messages for some actions.
type ActionMessageParams struct {
	Messages []EntityString `json:"messages"`
}

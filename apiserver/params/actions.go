// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	// TODO(jcw4) per fwereade 2014-11-21 remove this dependency
	"gopkg.in/juju/charm.v5"
)

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

// ActionResults is a slice of ActionResult for bulk requests.
type ActionResults struct {
	Results []ActionResult `json:"results,omitempty"`
}

// ActionResult describes an Action that will be or has been completed.
type ActionResult struct {
	Action    *Action                `json:"action,omitempty"`
	Enqueued  time.Time              `json:"enqueued,omitempty"`
	Started   time.Time              `json:"started,omitempty"`
	Completed time.Time              `json:"completed,omitempty"`
	Status    string                 `json:"status,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Output    map[string]interface{} `json:"output,omitempty"`
	Error     *Error                 `json:"error,omitempty"`
}

// ActionsByReceivers wrap a slice of Actions for API calls.
type ActionsByReceivers struct {
	Actions []ActionsByReceiver `json:"actions,omitempty"`
}

// ActionsByReceiver is a bulk API call wrapper containing Actions,
// either as input paramters or as results.
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

// ActionExecutionResults holds a slice of ActionExecutionResult for a
// bulk action API call
type ActionExecutionResults struct {
	Results []ActionExecutionResult `json:"results,omitempty"`
}

// ActionExecutionResult holds the action tag and output used when
// recording the result of an action.
type ActionExecutionResult struct {
	ActionTag string                 `json:"actiontag"`
	Status    string                 `json:"status"`
	Results   map[string]interface{} `json:"results,omitempty"`
	Message   string                 `json:"message,omitempty"`
}

// ServicesCharmActionsResults holds a slice of ServiceCharmActionsResult for
// a bulk result of charm Actions for Services.
type ServicesCharmActionsResults struct {
	Results []ServiceCharmActionsResult `json:"results,omitempty"`
}

// ServiceCharmActionsResult holds service name and charm.Actions for the service.
// If an error such as a missing charm or malformed service name occurs, it
// is encapsulated in this type.
type ServiceCharmActionsResult struct {
	ServiceTag string         `json:"servicetag,omitempty"`
	Actions    *charm.Actions `json:"actions,omitempty"`
	Error      *Error         `json:"error,omitempty"`
}

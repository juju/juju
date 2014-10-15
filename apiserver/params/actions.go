// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names"
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

	// ActionPending is the status of an Action that has been queued up
	// but not executed yet.
	ActionPending string = "pending"
)

// Actions is a slice of Action for bulk requests.
type Actions struct {
	Actions []Action `json"actions,omitempty"`
}

// Action describes an Action that will be or has been queued up.
type Action struct {
	Tag        names.ActionTag        `json:"tag"`
	Receiver   names.Tag              `json:"receiver"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// ActionResults is a slice of ActionResult for bulk requests.
type ActionResults struct {
	Results []ActionResult `json"results,omitempty"`
}

// ActionResult describes an ActionResult that will be or has been queued up.
type ActionResult struct {
	Action  *Action                `json:"action,omitempty"`
	Status  string                 `json:"status,omitempty"`
	Message string                 `json:"message,omitempty"`
	Output  map[string]interface{} `json:"output,omitempty"`
	Error   *Error                 `json:"error,omitempty"`
}

// Tags wrap a slice of names.Tag for API calls.
type Tags struct {
	Tags []names.Tag `json:"tags"`
}

// ActionsByReceivers wrap a slice of Actions for API calls.
type ActionsByReceivers struct {
	Actions []ActionsByReceiver `json:"actions,omitempty"`
}

// ActionsByReceiver is a bulk API call wrapper containing Actions,
// either as input paramters or as results.
type ActionsByReceiver struct {
	Receiver names.Tag      `json:"receiver,omitempty"`
	Actions  []ActionResult `json:"actions,omitempty"`
	Error    *Error         `json:"error,omitempty"`
}

// ActionTags are an array of ActionTag for bulk API calls
type ActionTags struct {
	Actions []names.ActionTag `json:"actions,omitempty"`
}

// ActionsQueryResults holds a slice of responses from the Actions
// query.
type ActionsQueryResults struct {
	Results []ActionsQueryResult `json:"results,omitempty"`
}

// ActionsQueryResult holds the name and parameters of an query result.
type ActionsQueryResult struct {
	Receiver names.Tag    `json:"receiver,omitempty"`
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
	ActionTag names.ActionTag        `json:"actiontag"`
	Status    string                 `json:"status"`
	Results   map[string]interface{} `json:"results,omitempty"`
	Message   string                 `json:"message,omitempty"`
}

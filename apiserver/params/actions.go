// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names"
)

// Action describes an Action that will be or has been queued up.
type Action struct {
	Tag        names.ActionTag        `json:"tag"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Status     string                 `json:"status,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Output     map[string]interface{} `json:"results,omitempty"`
}

// Tags wrap a slice of names.Tag for API calls.
type Tags struct {
	Tags []names.Tag `json:"tags"`
}

// ActionsByTag wrap a slice of Actions for API calls.
type ActionsByTag struct {
	Actions []Actions `json:"actions,omitempty"`
}

// Actions is a bulk API call wrapper containing Actions, either as
// input paramters or as results.
type Actions struct {
	Error    *Error    `json:"error,omitempty"`
	Receiver names.Tag `json:"receiver,omitempty"`
	Actions  []Action  `json:"actions,omitempty"`
}

// ActionTags are an array of ActionTag for bulk API calls
type ActionTags struct {
	Actions []names.ActionTag `json:"actiontags,omitempty"`
}

// ActionsQueryResults holds a slice of responses from the Actions
// query.
type ActionsQueryResults struct {
	Results []ActionsQueryResult `json:"results,omitempty"`
}

// ActionsQueryResult holds the name and parameters of an query result.
type ActionsQueryResult struct {
	Error    *Error    `json:"error,omitempty"`
	Receiver names.Tag `json:"receiver,omitempty"`
	Action   Action    `json:"result,omitempty"`
}

// ActionsRequest wraps a slice of Action's that represent Action
// definitions.
type ActionsRequest struct {
	Actions []Action `json:"actions,omitempty"`
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

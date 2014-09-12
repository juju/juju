// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"github.com/juju/names"
)

// ActionItem describes an Action that will be or has been queued up.
type ActionItem struct {
	UUID       string                 `json:"uuid"`
	Name       string                 `json:"name"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	Status     string                 `json:"status,omitempty"`
	Message    string                 `json:"message,omitempty"`
	Output     map[string]interface{} `json:"results,omitempty"`
}

// Actions is a bulk API call wrapper containing ActionItems, either
// as input paramters or as results.
type Actions struct {
	ActionItems []ActionItem `json:"actions,omitempty"`
}

// ActionTags are an array of ActionTag for bulk API calls
type ActionTags struct {
	Actions []names.ActionTag `json:"actiontags,omitempty"`
}

// ActionsQueryResults holds a slice of responses from the Actions query.
type ActionsQueryResults struct {
	Results []ActionsQueryResult `json:"results,omitempty"`
}

// ActionsQueryResult holds the name and parameters of an
// Actions query result.
type ActionsQueryResult struct {
	Error  *Error     `json:"error,omitempty"`
	Action ActionItem `json:"result,omitempty"`
}

// ActionsRequest is the wrapper struct for a bulk API call taking
// []Action
type ActionsRequest struct {
	Actions []ActionItem `json:"actions,omitempty"`
}

// ActionExecutionResults holds a slice of ActionExecutionResult for a bulk action API call
type ActionExecutionResults struct {
	Results []ActionExecutionResult `json:"results,omitempty"`
}

// ActionExecutionResult holds the action tag and output used when recording the
// result of an action.
type ActionExecutionResult struct {
	ActionTag string                 `json:"actiontag"`
	Status    string                 `json:"status"`
	Results   map[string]interface{} `json:"results,omitempty"`
	Message   string                 `json:"message,omitempty"`
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// Action represents a single instance of an Action call, by name and params.
type Action struct {
	id             string
	name           string
	params         map[string]interface{}
	parallel       bool
	executionGroup string
}

// NewAction makes a new Action with specified id, name and params.
func NewAction(id, name string, params map[string]interface{}, parallel bool, executionGroup string) *Action {
	return &Action{id: id, name: name, params: params, parallel: parallel, executionGroup: executionGroup}
}

// ID retrieves the ID of the Action.
func (a *Action) ID() string {
	return a.id
}

// Name retrieves the name of the Action.
func (a *Action) Name() string {
	return a.name
}

// Params retrieves the params map of the Action.
func (a *Action) Params() map[string]interface{} {
	return a.params
}

// Parallel returns true if the action can run without
// needed to acquire the machine lock.
func (a *Action) Parallel() bool {
	return a.parallel
}

// ExecutionGroup is the group of actions which cannot
// execute in parallel with each other.
func (a *Action) ExecutionGroup() string {
	return a.executionGroup
}

// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// Action represents a single instance of an Action call, by name and params.
type Action struct {
	name           string
	params         map[string]interface{}
	parallel       bool
	executionGroup string
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

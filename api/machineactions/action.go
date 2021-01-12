// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

// Action represents a single instance of an Action call, by name and params.
// TODO(bogdantelega): This is currently copied from uniter.Actions,
// but until the implementations converge, it's saner to duplicate the code since
// the "correct" abstraction over both is not obvious.
type Action struct {
	name           string
	params         map[string]interface{}
	parallel       bool
	executionGroup string
}

// NewAction makes a new Action with specified name and params map.
func NewAction(name string, params map[string]interface{}, parallel bool, executionGroup string) *Action {
	return &Action{name: name, params: params, parallel: parallel, executionGroup: executionGroup}
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

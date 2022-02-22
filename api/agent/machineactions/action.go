// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

// Action represents a single instance of an Action call, by name and params.
// TODO(bogdantelega): This is currently copied from uniter.Actions,
// but until the implementations converge, it's saner to duplicate the code since
// the "correct" abstraction over both is not obvious.
type Action struct {
	name   string
	params map[string]interface{}
}

// NewAction makes a new Action with specified name and params map.
func NewAction(name string, params map[string]interface{}) *Action {
	return &Action{name: name, params: params}
}

// Name retrieves the name of the Action.
func (a *Action) Name() string {
	return a.name
}

// Params retrieves the params map of the Action.
func (a *Action) Params() map[string]interface{} {
	return a.params
}

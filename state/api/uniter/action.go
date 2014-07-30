// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

// Action represents a single instance of an Action call, by name and params.
type Action struct {
	name   string
	params map[string]interface{}
}

// NewAction makes a new Action with specified name and params map.
func NewAction(name string, params map[string]interface{}) (*Action, error) {
	return &Action{name: name, params: params}, nil
}

// Name retrieves the name of the Action.
func (a *Action) Name() string {
	return a.name
}

// Params retrieves the params map of the Action.
func (a *Action) Params() map[string]interface{} {
	return a.params
}
